package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLargeSignedDashboardPayloadTriggers(t *testing.T) {
	oldBody := "- [ ] update\n" + strings.Repeat("dashboard content\n", 12000)
	newBody := "- [x] update\n" + strings.Repeat("dashboard content\n", 12000)
	payload := fmt.Sprintf(`{"action":"edited","sender":{"login":"person","type":"User"},"issue":{"title":"Renovate Dashboard 🤖","body":%q},"changes":{"body":{"from":%q}}}`, newBody, oldBody)
	if len(payload) <= 128<<10 {
		t.Fatalf("payload is only %d bytes", len(payload))
	}
	called := false
	s := &server{secret: []byte("secret"), botLogin: "bot", maxBody: 4 << 20, trigger: func(context.Context, triggerInfo) error { called = true; return nil }}
	req := httptest.NewRequest(http.MethodPost, "/hooks/renovate-dependency-dashboard", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "abc")
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(payload))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response := httptest.NewRecorder()
	s.handle(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !called {
		t.Fatal("trigger was not called")
	}
}

func TestRejectsInvalidSignature(t *testing.T) {
	s := &server{secret: []byte("secret"), maxBody: 4 << 20, trigger: func(context.Context, triggerInfo) error { t.Fatal("trigger called"); return nil }}
	req := httptest.NewRequest(http.MethodPost, "/hooks/renovate-dependency-dashboard", strings.NewReader(`{}`))
	req.Header.Set("X-Hub-Signature-256", "sha256=00")
	response := httptest.NewRecorder()
	s.handle(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestCheckboxChecked(t *testing.T) {
	if !checkboxChecked("text\n- [ ] update", "text\n- [x] update") {
		t.Fatal("expected checkbox transition")
	}
	if checkboxChecked("text\n- [x] update", "text\n- [ ] update") {
		t.Fatal("unchecked transition matched")
	}
}
