package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/diagnostic"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestHealthz(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type = %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("healthz body = %v, want status ok", body)
	}
}

func TestReadyz(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/readyz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz status = %d, want 200", resp.StatusCode)
	}
}

func TestInfoEndpoint(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/api/v1/info")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("info status = %d, want 200", resp.StatusCode)
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	var agent map[string]any
	if err := json.Unmarshal(body["agent"], &agent); err != nil {
		t.Fatalf("decode agent: %v", err)
	}
	if agent["version"] == "" || agent["go_version"] == "" {
		t.Errorf("agent info incomplete: %v", agent)
	}

	var instance map[string]any
	if err := json.Unmarshal(body["instance"], &instance); err != nil {
		t.Fatalf("decode instance: %v", err)
	}
	if instance["instance_id"] != "i-0123456789abcdef0" {
		t.Errorf("instance id = %v, want i-0123456789abcdef0", instance["instance_id"])
	}
	if instance["private_ip"] != "10.0.1.42" {
		t.Errorf("private ip = %v", instance["private_ip"])
	}
}

func TestStatusEndpoint(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/api/v1/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint = %d, want 200", resp.StatusCode)
	}

	var summary map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary["status"] != "pass" {
		t.Errorf("summary status = %v, want pass", summary["status"])
	}
	if summary["total"] != float64(7) {
		t.Errorf("summary total = %v, want 7", summary["total"])
	}
}

func TestNetworkEndpoint(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/api/v1/network")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("network endpoint = %d, want 200", resp.StatusCode)
	}

	var network map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&network); err != nil {
		t.Fatalf("decode network: %v", err)
	}
	if network["default_gateway"] != "10.0.2.2" {
		t.Errorf("gateway = %q, want 10.0.2.2", network["default_gateway"])
	}
	if network["default_interface"] != "eth0" {
		t.Errorf("interface = %q, want eth0", network["default_interface"])
	}
}

func TestProbeEndpoint(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/api/v1/probe")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe endpoint = %d, want 200", resp.StatusCode)
	}

	var report map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report["status"] != "pass" {
		t.Errorf("report status = %v, want pass", report["status"])
	}
	results, ok := report["results"].([]any)
	if !ok || len(results) != 7 {
		t.Errorf("report results = %v, want 7 entries", report["results"])
	}
}

func TestReportEndpointJSON(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/api/v1/report")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("report endpoint = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type = %q, want json", ct)
	}

	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	for _, key := range []string{"agent", "instance", "network", "summary", "probes", "diagnostics"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("report missing section %q", key)
		}
	}
}

func TestReportEndpointFormats(t *testing.T) {
	tests := []struct {
		query   string
		ct      string
		content string
	}{
		{query: "?format=markdown", ct: "text/markdown", content: "Evidence Report"},
		{query: "?format=text", ct: "text/plain", content: "EVIDENCE REPORT"},
		{query: "?format=unknown", ct: "application/json", content: "agent"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			_, _, server := newTestServer(t, nil)
			resp := get(t, server.URL+"/api/v1/report"+tt.query)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, tt.ct) {
				t.Errorf("content type = %q, want prefix %q", ct, tt.ct)
			}
			body := new(strings.Builder)
			_, _ = io.Copy(body, resp.Body)
			if !strings.Contains(body.String(), tt.content) {
				t.Errorf("body missing %q", tt.content)
			}
		})
	}
}

func TestEchoEndpoint(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/echo", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("User-Agent", "vpc-proof-test-agent")
	req.Header.Set("X-Forwarded-For", "198.51.100.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("echo request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("echo status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode echo: %v", err)
	}
	if body["ip"] != "198.51.100.9" {
		t.Errorf("ip = %v, want X-Forwarded-For value", body["ip"])
	}
	if body["user_agent"] != "vpc-proof-test-agent" {
		t.Errorf("user_agent = %v", body["user_agent"])
	}
	if _, err := time.Parse(time.RFC3339, body["received_at"].(string)); err != nil {
		t.Errorf("received_at not RFC3339: %v", err)
	}
}

func TestEchoEndpointFallsBackToRemoteAddr(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/api/v1/echo")
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode echo: %v", err)
	}
	if body["ip"] != "127.0.0.1" {
		t.Errorf("ip = %v, want 127.0.0.1 (direct connection)", body["ip"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	// Generate one probe run so request counters and probe gauges exist.
	_ = get(t, server.URL+"/api/v1/probe")

	resp := get(t, server.URL+"/metrics")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", resp.StatusCode)
	}

	body := new(strings.Builder)
	_, _ = io.Copy(body, resp.Body)
	for _, want := range []string{
		"# TYPE vpc_proof_http_requests_total counter",
		`vpc_proof_http_requests_total{method="GET",path="/api/v1/probe",status="200"}`,
		"# TYPE vpc_proof_http_request_duration_seconds histogram",
		"# TYPE vpc_proof_probe_status gauge",
		`vpc_proof_probe_status{probe="metadata",status="pass"}`,
	} {
		if !strings.Contains(body.String(), want) {
			t.Errorf("metrics output missing %q:\n%s", want, body.String())
		}
	}
}

func TestUnknownRoute(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/api/v1/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown route status = %d, want 404", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] == nil || body["request_id"] == nil || body["timestamp"] == nil {
		t.Errorf("error body incomplete: %v", body)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp, err := http.Post(server.URL+"/healthz", "text/plain", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /healthz status = %d, want 405", resp.StatusCode)
	}
}

func TestReadyzNotReady(t *testing.T) {
	// A Handlers with no dependencies wired must report not ready.
	handlers := &Handlers{}
	ts := httptest.NewServer(handlers.routes())
	t.Cleanup(ts.Close)

	resp := get(t, ts.URL+"/readyz")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503 when not wired", resp.StatusCode)
	}
}

func TestNewValidation(t *testing.T) {
	cfg := config.Defaults()
	if _, err := New(Options{}); err == nil {
		t.Fatal("expected error without config")
	}
	if _, err := New(Options{Config: cfg}); err == nil {
		t.Fatal("expected error without runner")
	}
	if _, err := New(Options{Config: cfg, Runner: probe.NewRunner(nil)}); err == nil {
		t.Fatal("expected error without engine")
	}
	if _, err := New(Options{Config: cfg, Runner: probe.NewRunner(nil), Engine: diagnostic.New()}); err == nil {
		t.Fatal("expected error without cache")
	}
}

func TestServerListenAndServeShutdown(t *testing.T) {
	port := freeAPIPort(t)
	server, _, _ := newTestServer(t, func(cfg *config.Config) {
		cfg.Server.Addr = "127.0.0.1"
		cfg.Server.Port = port
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	waitAPIPort(t, port)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("ListenAndServe returned unexpected error: %v", err)
	}
}

func TestSecurityHeaders(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/healthz")
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options: nosniff")
	}
	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Error("missing X-Frame-Options: DENY")
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("missing X-Request-ID header")
	}
}
