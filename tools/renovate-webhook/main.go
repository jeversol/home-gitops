package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxBodyDefault = int64(4 << 20)

type event struct {
	Action      string                       `json:"action"`
	Sender      struct{ Login, Type string } `json:"sender"`
	Issue       struct{ Title, Body string } `json:"issue"`
	PullRequest struct {
		Number    int               `json:"number"`
		Title     string            `json:"title"`
		Body      string            `json:"body"`
		HTMLURL   string            `json:"html_url"`
		Merged    bool              `json:"merged"`
		Assignees []json.RawMessage `json:"assignees"`
		Head      struct {
			Ref string `json:"ref"`
		} `json:"head"`
	} `json:"pull_request"`
	Changes struct {
		Body struct {
			From *string `json:"from"`
		} `json:"body"`
	} `json:"changes"`
}

type triggerInfo struct{ Delivery, Event, Action, Sender string }
type server struct {
	secret                                  []byte
	botLogin                                string
	maxBody                                 int64
	trigger                                 func(context.Context, triggerInfo) error
	httpClient                              *http.Client
	pushoverToken, pushoverUser, discordURL string
}

func main() {
	secret := os.Getenv("GITHUB_SECRET")
	if secret == "" {
		slog.Error("GITHUB_SECRET is required")
		os.Exit(1)
	}
	kube, err := newKubeClient()
	if err != nil {
		slog.Error("initialize Kubernetes client", "error", err)
		os.Exit(1)
	}
	s := &server{
		secret: []byte(secret), botLogin: env("RENOVATE_BOT_LOGIN", "harry-botter-lumos[bot]"),
		maxBody: envInt("MAX_WEBHOOK_BODY_BYTES", maxBodyDefault), trigger: kube.trigger,
		httpClient: &http.Client{Timeout: 10 * time.Second}, pushoverToken: os.Getenv("PUSHOVER_API_TOKEN"),
		pushoverUser: os.Getenv("PUSHOVER_USER_KEY"), discordURL: os.Getenv("DISCORD_WEBHOOK_URL"),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /hooks/renovate-dependency-dashboard", s.handle)
	h := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	slog.Info("webhook listening", "address", h.Addr, "maxBodyBytes", s.maxBody)
	if err := h.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	delivery, eventName := r.Header.Get("X-GitHub-Delivery"), r.Header.Get("X-GitHub-Event")
	logger := slog.With("delivery", delivery, "event", eventName)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxBody))
	if err != nil {
		logger.Warn("invalid body", "error", err)
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !validSignature(body, r.Header.Get("X-Hub-Signature-256"), s.secret) {
		logger.Warn("invalid signature")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	var e event
	if err := json.Unmarshal(body, &e); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if e.Sender.Type != "User" && e.Sender.Type != "Bot" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	trigger := false
	switch eventName {
	case "issues":
		if e.Sender.Login == s.botLogin || e.Action != "edited" || (e.Issue.Title != "Dependency Dashboard" && e.Issue.Title != "Renovate Dashboard 🤖") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		trigger = e.Changes.Body.From != nil && checkboxChecked(*e.Changes.Body.From, e.Issue.Body)
	case "pull_request":
		if !strings.HasPrefix(e.PullRequest.Head.Ref, "renovate/") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch e.Action {
		case "opened":
			if len(e.PullRequest.Assignees) > 0 {
				s.notify(r.Context(), e.PullRequest.Number, e.PullRequest.Title, e.PullRequest.HTMLURL, logger)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		case "closed":
			trigger = e.PullRequest.Merged
		case "edited":
			trigger = e.Sender.Login != s.botLogin && e.Changes.Body.From != nil && checkboxChecked(*e.Changes.Body.From, e.PullRequest.Body)
		default:
			w.WriteHeader(http.StatusNoContent)
			return
		}
	default:
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !trigger {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	err = s.trigger(r.Context(), triggerInfo{delivery, eventName, e.Action, e.Sender.Type})
	if errors.Is(err, errJobRunning) {
		logger.Info("Renovate job already running")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		logger.Error("trigger job", "error", err)
		http.Error(w, "failed to trigger Renovate", http.StatusInternalServerError)
		return
	}
	logger.Info("Renovate job triggered", "action", e.Action, "sender", e.Sender.Login)
	w.WriteHeader(http.StatusNoContent)
}

func validSignature(body []byte, header string, secret []byte) bool {
	if !strings.HasPrefix(header, "sha256=") {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func checkboxChecked(oldBody, newBody string) bool {
	oldLines, newLines := strings.Split(oldBody, "\n"), strings.Split(newBody, "\n")
	for i := 0; i < min(len(oldLines), len(newLines)); i++ {
		o, n := strings.ToLower(strings.TrimSpace(oldLines[i])), strings.ToLower(strings.TrimSpace(newLines[i]))
		unchecked := strings.HasPrefix(o, "- [ ]") || strings.HasPrefix(o, "* [ ]")
		checked := strings.HasPrefix(n, "- [x]") || strings.HasPrefix(n, "* [x]") || strings.HasPrefix(n, "- [*]") || strings.HasPrefix(n, "* [*]")
		if o != n && unchecked && checked {
			return true
		}
	}
	return false
}

func (s *server) notify(ctx context.Context, number int, title, link string, logger *slog.Logger) {
	if s.httpClient == nil {
		return
	}
	if s.pushoverToken != "" && s.pushoverUser != "" {
		form := url.Values{"token": {s.pushoverToken}, "user": {s.pushoverUser}, "title": {fmt.Sprintf("Renovate PR #%d", number)}, "message": {title}, "url": {link}, "url_title": {"View PR"}, "priority": {"0"}}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.pushover.net/1/messages.json", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		doNotify(s.httpClient, req, "Pushover", logger)
	}
	if s.discordURL != "" {
		payload, _ := json.Marshal(map[string]string{"content": fmt.Sprintf("**Renovate PR #%d**\n%s\n%s", number, title, link)})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.discordURL, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		doNotify(s.httpClient, req, "Discord", logger)
	}
}

func doNotify(client *http.Client, req *http.Request, name string, logger *slog.Logger) {
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn(name+" notification failed", "error", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger.Warn(name+" notification failed", "status", resp.StatusCode)
	}
}

var errJobRunning = errors.New("Renovate webhook job already running")

type kubeClient struct {
	client                            *http.Client
	server, namespace, cronJob, token string
	ttl                               int
}

func newKubeClient() (*kubeClient, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("Kubernetes service environment is unavailable")
	}
	token, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid Kubernetes CA bundle")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	return &kubeClient{client: &http.Client{Transport: transport, Timeout: 15 * time.Second}, server: "https://" + host + ":" + port,
		namespace: env("CRONJOB_NAMESPACE", "renovate"), cronJob: env("CRONJOB_NAME", "renovate"), token: strings.TrimSpace(string(token)), ttl: int(envInt("RENOVATE_JOB_TTL_SECONDS", 900))}, nil
}

func (k *kubeClient) request(ctx context.Context, method, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, k.server+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+k.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := k.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Kubernetes API status %d: %s", resp.StatusCode, strings.TrimSpace(string(response)))
	}
	if out != nil {
		return json.Unmarshal(response, out)
	}
	return nil
}

func (k *kubeClient) trigger(ctx context.Context, info triggerInfo) error {
	base := "/apis/batch/v1/namespaces/" + url.PathEscape(k.namespace)
	var jobs struct {
		Items []struct {
			Metadata struct {
				DeletionTimestamp any `json:"deletionTimestamp"`
			} `json:"metadata"`
			Status struct {
				Active         int `json:"active"`
				CompletionTime any `json:"completionTime"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := k.request(ctx, http.MethodGet, base+"/jobs?labelSelector=recompiled.org%2Frenovate-trigger%3Dwebhook", nil, &jobs); err != nil {
		return err
	}
	for _, job := range jobs.Items {
		if job.Metadata.DeletionTimestamp == nil && (job.Status.Active > 0 || job.Status.CompletionTime == nil) {
			return errJobRunning
		}
	}
	var cron struct {
		Metadata struct {
			Namespace string            `json:"namespace"`
			Labels    map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			JobTemplate struct {
				Metadata struct{ Labels, Annotations map[string]string } `json:"metadata"`
				Spec     json.RawMessage                                 `json:"spec"`
			} `json:"jobTemplate"`
		} `json:"spec"`
	}
	if err := k.request(ctx, http.MethodGet, base+"/cronjobs/"+url.PathEscape(k.cronJob), nil, &cron); err != nil {
		return err
	}
	slug := strings.ToLower(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, info.Delivery))
	if len(slug) > 20 {
		slug = slug[:20]
	}
	if slug == "" {
		slug = time.Now().UTC().Format("20060102150405")
	}
	labels := cloneMap(cron.Metadata.Labels)
	for key, value := range cron.Spec.JobTemplate.Metadata.Labels {
		labels[key] = value
	}
	labels["recompiled.org/renovate-trigger"], labels["recompiled.org/renovate-source"] = "webhook", "dependency-dashboard"
	annotations := cloneMap(cron.Spec.JobTemplate.Metadata.Annotations)
	for key, value := range map[string]string{"recompiled.org/github-delivery": info.Delivery, "recompiled.org/github-event": info.Event, "recompiled.org/github-action": info.Action, "recompiled.org/github-sender": info.Sender} {
		if value != "" {
			annotations[key] = value
		}
	}
	spec := map[string]any{}
	if err := json.Unmarshal(cron.Spec.JobTemplate.Spec, &spec); err != nil {
		return err
	}
	if k.ttl > 0 {
		spec["ttlSecondsAfterFinished"] = k.ttl
	}
	job := map[string]any{"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]any{"name": "renovate-triggered-" + slug, "namespace": cron.Metadata.Namespace, "labels": labels, "annotations": annotations}, "spec": spec}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return k.request(ctx, http.MethodPost, base+"/jobs", payload, nil)
}

func cloneMap(input map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range input {
		result[key] = value
	}
	return result
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func envInt(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
