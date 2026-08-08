package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
)

func TestConfigEndpointRedactsToken(t *testing.T) {
	_, _, server := newTestServer(t, func(cfg *config.Config) {
		cfg.Auth.Enabled = true
		cfg.Auth.Token = "super-secret-token"
	})

	resp := doRequest(t, http.MethodGet, server.URL+"/api/v1/config", map[string]string{
		"Authorization": "Bearer super-secret-token",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config endpoint status = %d, want 200", resp.StatusCode)
	}

	body := readBody(t, resp)
	if strings.Contains(body, "super-secret-token") {
		t.Fatalf("auth token leaked into the config endpoint:\n%s", body)
	}
	if !strings.Contains(body, "[REDACTED]") {
		t.Errorf("auth token should be redacted:\n%s", body)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("config response is not valid JSON: %v", err)
	}
	auth, ok := doc["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth section missing: %v", doc)
	}
	if auth["enabled"] != true {
		t.Errorf("auth.enabled = %v, want true", auth["enabled"])
	}
	if auth["token"] != "[REDACTED]" {
		t.Errorf("auth.token = %v, want [REDACTED]", auth["token"])
	}
}

func TestOpenAPIEndpoint(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/api/v1/openapi.json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("openapi status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type = %q, want application/json", ct)
	}

	body := readBody(t, resp)
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("openapi document is not valid JSON: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Errorf("openapi version = %v, want 3.0.3", doc["openapi"])
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths missing from openapi document")
	}
	for _, path := range []string{
		"/healthz", "/readyz", "/metrics", "/api/v1/info", "/api/v1/status",
		"/api/v1/network", "/api/v1/probe", "/api/v1/report", "/api/v1/echo",
		"/api/v1/history", "/api/v1/config", "/api/v1/openapi.json",
	} {
		if _, ok := paths[path]; !ok {
			t.Errorf("openapi document missing path %q", path)
		}
	}
}

func TestHistoryEndpoint(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	// A fresh probe run populates the history store.
	_ = get(t, server.URL+"/api/v1/probe")

	resp := get(t, server.URL+"/api/v1/history")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d, want 200", resp.StatusCode)
	}

	var entries []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(entries))
	}
	if entries[0]["status"] != "pass" {
		t.Errorf("history entry status = %v, want pass", entries[0]["status"])
	}
	if entries[0]["total"] != nil {
		t.Errorf("history entries should not carry a total field: %v", entries[0])
	}
}

func TestHistoryEndpointEmpty(t *testing.T) {
	handlers := &Handlers{}
	ts := httptest.NewServer(handlers.routes())
	t.Cleanup(ts.Close)

	resp := get(t, ts.URL+"/api/v1/history")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d, want 200", resp.StatusCode)
	}
	if body := readBody(t, resp); !strings.Contains(body, "[]") {
		t.Errorf("empty history should be an empty array, got %s", body)
	}
}
