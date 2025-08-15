package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"talos-automation/internal/repo"
	"talos-automation/internal/talos"
	"talos-automation/upgrades"
)

type GitHubWebhook struct {
	Action string `json:"action"`
	Ref    string `json:"ref"`
	Commits []struct {
		Modified []string `json:"modified"`
	} `json:"commits"`
	Repository struct {
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
}

type Config struct {
	WebhookSecret    string
	TalosConfigPath  string
	LogPath          string
	GitHubToken      string
	GitHubOwner      string
	GitHubRepo       string
	Port             string
	DiagnosticsToken string
}

func loadConfig() *Config {
	return &Config{
		WebhookSecret:    os.Getenv("GITHUB_WEBHOOK_SECRET"),
		TalosConfigPath:  os.Getenv("TALOS_CONFIG_PATH"),
		LogPath:          os.Getenv("LOG_PATH"),
		GitHubToken:      os.Getenv("GITHUB_TOKEN"),
		GitHubOwner:      os.Getenv("GITHUB_OWNER"),
		GitHubRepo:       os.Getenv("GITHUB_REPO"),
		Port:             getEnvWithDefault("PORT", "3847"),
		DiagnosticsToken: os.Getenv("DIAGNOSTICS_TOKEN"),
	}
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func verifyWebhookSignature(payload []byte, signature string, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	expectedMAC := hmac.New(sha256.New, []byte(secret))
	expectedMAC.Write(payload)
	expectedSignature := "sha256=" + hex.EncodeToString(expectedMAC.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func handleWebhook(w http.ResponseWriter, r *http.Request, config *Config) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	if !verifyWebhookSignature(payload, signature, config.WebhookSecret) {
		log.Printf("Invalid webhook signature")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var webhook GitHubWebhook
	if err := json.Unmarshal(payload, &webhook); err != nil {
		log.Printf("Error parsing webhook payload: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Only process push events to main branch
	if webhook.Ref != "refs/heads/main" {
		log.Printf("Ignoring push to branch: %s", webhook.Ref)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check if track-versions.yaml was modified
	trackVersionsModified := false
	for _, commit := range webhook.Commits {
		for _, file := range commit.Modified {
			if file == "infrastructure/cluster/track-versions.yaml" {
				trackVersionsModified = true
				break
			}
		}
		if trackVersionsModified {
			break
		}
	}

	if !trackVersionsModified {
		log.Printf("track-versions.yaml not modified, ignoring webhook")
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("track-versions.yaml modified, processing upgrade...")
	
	if err := processUpgrade(config); err != nil {
		log.Printf("Upgrade failed: %v", err)
		http.Error(w, "Upgrade failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Upgrade processed successfully")
}

func processUpgradeWithTestOverrides(cfg *Config, currentK8s, currentTalos, scenario string) error {
	log.Printf("Fetching current versions from repository...")
	
	// Create GitHub client and fetch versions
	githubClient := repo.NewGitHubClient(cfg.GitHubToken)
	versions, err := githubClient.FetchVersions(cfg.GitHubOwner, cfg.GitHubRepo)
	if err != nil {
		return fmt.Errorf("failed to fetch versions: %w", err)
	}

	log.Printf("Found versions - Talos: %s, Kubernetes: %s", versions.TalosVersion, versions.KubernetesVersion)

	// Parse talos config to get node information
	talosConfig, err := talos.ParseConfig(cfg.TalosConfigPath)
	if err != nil {
		return fmt.Errorf("failed to parse talos config: %w", err)
	}

	allNodes, err := talosConfig.GetAllNodes()
	if err != nil {
		return fmt.Errorf("failed to get cluster nodes: %w", err)
	}
	
	controlPlaneNode, err := talosConfig.GetFirstControlPlaneNode()
	if err != nil {
		return fmt.Errorf("failed to get control plane node: %w", err)
	}

	log.Printf("Using control plane node: %s", controlPlaneNode)
	log.Printf("Cluster has %d nodes: %v", len(allNodes), allNodes)

	// Step 1: Upgrade Talos if needed (must be done first)
	log.Printf("Checking for Talos upgrade...")
	talosUpgrader := upgrades.NewTalosUpgrader(cfg.TalosConfigPath, cfg.LogPath, githubClient)
	
	// Apply test overrides for Talos
	if currentTalos != "" {
		talosUpgrader.SetMockCurrentVersion(currentTalos)
		log.Printf("TEST OVERRIDE: Using Talos version %s", currentTalos)
	} else {
		switch scenario {
		case "talos-upgrade", "both-upgrade":
			talosUpgrader.SetMockCurrentVersion("1.10.5")
			log.Printf("TEST SCENARIO: Using Talos version 1.10.5")
		}
	}
	
	if err := talosUpgrader.UpgradeToVersion(versions.TalosVersion, allNodes, cfg.GitHubOwner, cfg.GitHubRepo, false); err != nil {
		return fmt.Errorf("talos upgrade failed: %w", err)
	}

	log.Printf("Talos upgrade completed successfully")

	// Step 2: Upgrade Kubernetes if needed
	log.Printf("Starting Kubernetes upgrade...")
	k8sUpgrader := upgrades.NewKubernetesUpgrader(cfg.TalosConfigPath, cfg.LogPath)
	
	// Apply test overrides for K8s
	if currentK8s != "" {
		k8sUpgrader.SetMockCurrentVersion(currentK8s)
		log.Printf("TEST OVERRIDE: Using K8s version %s", currentK8s)
	} else {
		switch scenario {
		case "k8s-upgrade", "both-upgrade":
			k8sUpgrader.SetMockCurrentVersion("1.33.2")
			log.Printf("TEST SCENARIO: Using K8s version 1.33.2")
		}
	}
	
	if err := k8sUpgrader.UpgradeToVersion(versions.KubernetesVersion, controlPlaneNode, false); err != nil {
		return fmt.Errorf("kubernetes upgrade failed: %w", err)
	}

	log.Printf("All upgrades completed successfully")
	return nil
}

func processUpgrade(cfg *Config) error {
	log.Printf("Fetching current versions from repository...")
	
	// Create GitHub client and fetch versions
	githubClient := repo.NewGitHubClient(cfg.GitHubToken)
	versions, err := githubClient.FetchVersions(cfg.GitHubOwner, cfg.GitHubRepo)
	if err != nil {
		return fmt.Errorf("failed to fetch versions: %w", err)
	}

	log.Printf("Found versions - Talos: %s, Kubernetes: %s", versions.TalosVersion, versions.KubernetesVersion)

	// Parse talos config to get node information
	talosConfig, err := talos.ParseConfig(cfg.TalosConfigPath)
	if err != nil {
		return fmt.Errorf("failed to parse talos config: %w", err)
	}

	allNodes, err := talosConfig.GetAllNodes()
	if err != nil {
		return fmt.Errorf("failed to get cluster nodes: %w", err)
	}
	
	controlPlaneNode, err := talosConfig.GetFirstControlPlaneNode()
	if err != nil {
		return fmt.Errorf("failed to get control plane node: %w", err)
	}

	log.Printf("Using control plane node: %s", controlPlaneNode)
	log.Printf("Cluster has %d nodes: %v", len(allNodes), allNodes)

	// Step 1: Upgrade Talos if needed (must be done first)
	log.Printf("Checking for Talos upgrade...")
	talosUpgrader := upgrades.NewTalosUpgrader(cfg.TalosConfigPath, cfg.LogPath, githubClient)
	
	if err := talosUpgrader.UpgradeToVersion(versions.TalosVersion, allNodes, cfg.GitHubOwner, cfg.GitHubRepo, true); err != nil {
		return fmt.Errorf("talos upgrade failed: %w", err)
	}

	log.Printf("Talos upgrade completed successfully")

	// Step 2: Upgrade Kubernetes if needed
	log.Printf("Starting Kubernetes upgrade...")
	k8sUpgrader := upgrades.NewKubernetesUpgrader(cfg.TalosConfigPath, cfg.LogPath)
	
	if err := k8sUpgrader.UpgradeToVersion(versions.KubernetesVersion, controlPlaneNode, true); err != nil {
		return fmt.Errorf("kubernetes upgrade failed: %w", err)
	}

	log.Printf("All upgrades completed successfully")
	return nil
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func diagnosticsEndpoint(w http.ResponseWriter, r *http.Request, config *Config) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check Bearer token authorization
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		log.Printf("Diagnostics endpoint accessed without Authorization header")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		log.Printf("Diagnostics endpoint accessed with invalid Authorization header format")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != config.DiagnosticsToken {
		log.Printf("Diagnostics endpoint accessed with invalid token")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	log.Printf("Diagnostics endpoint accessed with valid token")

	w.Header().Set("Content-Type", "application/json")
	
	// Check for scenario parameter to simulate different conditions
	scenario := r.URL.Query().Get("scenario")
	currentK8s := r.URL.Query().Get("current_k8s")
	currentTalos := r.URL.Query().Get("current_talos")
	
	results := make(map[string]interface{})
	
	// Test 1: GitHub API
	log.Printf("Testing GitHub API connection...")
	githubClient := repo.NewGitHubClient(config.GitHubToken)
	versions, err := githubClient.FetchVersions(config.GitHubOwner, config.GitHubRepo)
	if err != nil {
		results["github_api"] = map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
		}
	} else {
		results["github_api"] = map[string]interface{}{
			"status":             "success",
			"talos_version":      versions.TalosVersion,
			"kubernetes_version": versions.KubernetesVersion,
		}
	}

	// Test 2: Talos Config Parsing
	log.Printf("Testing Talos config parsing...")
	talosConfig, err := talos.ParseConfig(config.TalosConfigPath)
	if err != nil {
		results["talos_config"] = map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
		}
	} else {
		controlPlaneNode, _ := talosConfig.GetFirstControlPlaneNode()
		allNodes, _ := talosConfig.GetAllNodes()
		results["talos_config"] = map[string]interface{}{
			"status":             "success",
			"control_plane_node": controlPlaneNode,
			"all_nodes":          allNodes,
			"node_count":         len(allNodes),
		}
	}

	// Test 3: Bare Metal Config
	log.Printf("Testing bare metal config fetch...")
	bareMetalConfig, err := githubClient.FetchBareMetalConfig(config.GitHubOwner, config.GitHubRepo)
	if err != nil {
		results["bare_metal_config"] = map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
		}
	} else {
		results["bare_metal_config"] = map[string]interface{}{
			"status": "success",
			"size":   len(bareMetalConfig),
		}
	}

	// Test 4: Image Factory API
	log.Printf("Testing Image Factory API...")
	if bareMetalConfig != nil {
		factory := upgrades.NewImageFactoryClient()
		schematicID, err := factory.CreateSchematic(bareMetalConfig)
		if err != nil {
			results["image_factory"] = map[string]interface{}{
				"status": "failed",
				"error":  err.Error(),
			}
		} else {
			results["image_factory"] = map[string]interface{}{
				"status":      "success",
				"schematic_id": schematicID,
			}
		}
	} else {
		results["image_factory"] = map[string]interface{}{
			"status": "skipped",
			"reason": "bare_metal_config_failed",
		}
	}

	// Test 5: Run the exact same upgrade logic as production, but with test mode
	if talosConfig != nil {
		log.Printf("Running IDENTICAL upgrade logic as production...")
		
		// Get real versions first for logging comparison
		tempK8sUpgrader := upgrades.NewKubernetesUpgrader(config.TalosConfigPath, config.LogPath)
		tempTalosUpgrader := upgrades.NewTalosUpgrader(config.TalosConfigPath, config.LogPath, githubClient)
		controlPlaneNode, _ := talosConfig.GetFirstControlPlaneNode()
		
		realK8sVersion, k8sErr := tempK8sUpgrader.GetCurrentVersion(controlPlaneNode, true)
		realTalosVersion, talosErr := tempTalosUpgrader.GetCurrentVersion(true)
		
		if k8sErr == nil {
			log.Printf("Detected K8s version: %s", realK8sVersion)
		} else {
			log.Printf("Could not get real K8s version: %v", k8sErr)
		}
		
		if talosErr == nil {
			log.Printf("Detected Talos version: %s", realTalosVersion)
		} else {
			log.Printf("Could not get real Talos version: %v", talosErr)
		}
		
		results["cluster_versions"] = map[string]interface{}{
			"status":               "success",
			"real_k8s_version":     realK8sVersion,
			"real_talos_version":   realTalosVersion,
		}
		
		// Make overrides explicit in logs if provided
		if currentK8s != "" && realK8sVersion != "" && currentK8s != realK8sVersion {
			log.Printf("Detected K8s version %s, overriding with %s from diagnostics call", realK8sVersion, currentK8s)
		}
		if currentTalos != "" && realTalosVersion != "" && currentTalos != realTalosVersion {
			log.Printf("Detected Talos version %s, overriding with %s from diagnostics call", realTalosVersion, currentTalos)
		}
		
		// Now run the exact same processUpgrade logic with test overrides
		err := processUpgradeWithTestOverrides(config, currentK8s, currentTalos, scenario)
		if err != nil {
			results["upgrade_test"] = map[string]interface{}{
				"status": "failed",
				"error":  err.Error(),
			}
		} else {
			results["upgrade_test"] = map[string]interface{}{
				"status":  "success",
				"message": "Full upgrade logic validated successfully",
			}
		}
	}

	// Test 6: Upgrade Decision Logic
	if versions != nil && talosConfig != nil {
		log.Printf("Testing upgrade decision logic...")
		controlPlaneNode, _ := talosConfig.GetFirstControlPlaneNode()
		allNodes, _ := talosConfig.GetAllNodes()
		
		results["upgrade_decisions"] = map[string]interface{}{
			"would_upgrade_kubernetes": versions.KubernetesVersion,
			"would_upgrade_talos":      versions.TalosVersion,
			"target_control_plane":     controlPlaneNode,
			"target_nodes":             allNodes,
		}
	}

	// Summary
	results["summary"] = map[string]interface{}{
		"timestamp":     fmt.Sprintf("%d", time.Now().Unix()),
		"ready":         checkAllTestsPass(results),
		"scenario":      scenario,
		"current_k8s":   currentK8s,
		"current_talos": currentTalos,
	}

	json.NewEncoder(w).Encode(results)
}

func checkAllTestsPass(results map[string]interface{}) bool {
	tests := []string{"github_api", "talos_config", "bare_metal_config", "image_factory", "cluster_versions", "k8s_upgrade_test", "talos_upgrade_test"}
	
	for _, test := range tests {
		if testResult, exists := results[test]; exists {
			if resultMap, ok := testResult.(map[string]interface{}); ok {
				if status, hasStatus := resultMap["status"]; hasStatus {
					if status != "success" {
						return false
					}
				}
			}
		}
	}
	return true
}

func main() {
	config := loadConfig()

	// Validate required environment variables
	if config.WebhookSecret == "" {
		log.Fatal("GITHUB_WEBHOOK_SECRET environment variable is required")
	}
	if config.GitHubToken == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}
	if config.GitHubOwner == "" {
		log.Fatal("GITHUB_OWNER environment variable is required")
	}
	if config.GitHubRepo == "" {
		log.Fatal("GITHUB_REPO environment variable is required")
	}
	if config.TalosConfigPath == "" {
		log.Fatal("TALOS_CONFIG_PATH environment variable is required")
	}
	if config.LogPath == "" {
		log.Fatal("LOG_PATH environment variable is required")
	}
	if config.DiagnosticsToken == "" {
		log.Fatal("DIAGNOSTICS_TOKEN environment variable is required")
	}

	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		handleWebhook(w, r, config)
	})
	
	http.HandleFunc("/health", healthCheck)
	
	http.HandleFunc("/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		diagnosticsEndpoint(w, r, config)
	})

	log.Printf("Starting talos-automation webhook server on port %s", config.Port)
	log.Fatal(http.ListenAndServe(":"+config.Port, nil))
}
