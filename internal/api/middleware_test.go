package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

func TestAuthMiddleware(t *testing.T) {
	_, _, server := newTestServer(t, func(cfg *config.Config) {
		cfg.Auth.Enabled = true
		cfg.Auth.Token = "secret-token"
	})

	t.Run("public path bypasses auth", func(t *testing.T) {
		resp := get(t, server.URL+"/healthz")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("healthz status = %d, want 200 (public path)", resp.StatusCode)
		}
	})

	t.Run("missing token rejected", func(t *testing.T) {
		resp := get(t, server.URL+"/api/v1/echo")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
		if resp.Header.Get("WWW-Authenticate") == "" {
			t.Error("missing WWW-Authenticate header")
		}
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		resp := doRequest(t, http.MethodGet, server.URL+"/api/v1/echo", map[string]string{
			"Authorization": "Bearer wrong-token",
		})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("malformed authorization header rejected", func(t *testing.T) {
		resp := doRequest(t, http.MethodGet, server.URL+"/api/v1/echo", map[string]string{
			"Authorization": "Basic dXNlcjpwYXNz",
		})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("valid token accepted", func(t *testing.T) {
		resp := doRequest(t, http.MethodGet, server.URL+"/api/v1/echo", map[string]string{
			"Authorization": "Bearer secret-token",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})
}

func TestUnauthorizedBodyCarriesRequestID(t *testing.T) {
	_, _, server := newTestServer(t, func(cfg *config.Config) {
		cfg.Auth.Enabled = true
		cfg.Auth.Token = "secret-token"
	})

	resp := get(t, server.URL+"/api/v1/echo")
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["request_id"] == nil || body["request_id"] == "" {
		t.Errorf("error body missing request_id: %v", body)
	}
	if body["timestamp"] == nil {
		t.Errorf("error body missing timestamp: %v", body)
	}
	if body["error"] == nil {
		t.Errorf("error body missing error message: %v", body)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	logger, err := observability.New("info", "json", io.Discard)
	if err != nil {
		t.Fatalf("observability.New: %v", err)
	}
	t.Cleanup(func() { _ = logger.Sync() })

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})
	handler := chain(panicking, withRequestID, recoverMiddleware(logger))

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	resp := get(t, ts.URL+"/anything")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("panic response status = %d, want 500", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "internal server error" {
		t.Errorf("error message = %v", body["error"])
	}
	if body["request_id"] == "" {
		t.Errorf("error body should carry the request id: %v", body)
	}
	if strings.Contains(readBody(t, resp), "boom") {
		t.Error("panic details leaked into the response")
	}
}

func TestRequestIDHeader(t *testing.T) {
	_, _, server := newTestServer(t, nil)

	resp := get(t, server.URL+"/api/v1/echo")
	id1 := resp.Header.Get("X-Request-ID")
	if id1 == "" {
		t.Fatal("missing X-Request-ID header")
	}

	resp2 := get(t, server.URL+"/api/v1/echo")
	id2 := resp2.Header.Get("X-Request-ID")
	if id1 == id2 {
		t.Error("request IDs should be unique across requests")
	}
}
