package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
)

func TestEchoClientCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/echo" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"ip":"203.0.113.7","user_agent":"test-agent","received_at":"2026-08-08T12:00:00Z"}`)
	}))
	t.Cleanup(server.Close)

	deps, _ := defaultDeps(t)
	stdout, _, code := runCLIWith(&deps, "echo-client", "--url", server.URL)

	if code != exitCodeOK {
		t.Fatalf("echo-client exit code = %d, want 0", code)
	}
	for _, want := range []string{"IP          : 203.0.113.7", "User-Agent  : test-agent", "Received at : 2026-08-08T12:00:00Z"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("echo-client output missing %q, got:\n%s", want, stdout)
		}
	}
}

func TestEchoClientCommandError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/echo" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	deps, _ := defaultDeps(t)
	_, stderr, code := runCLIWith(&deps, "echo-client", "--url", server.URL)
	if code == exitCodeOK {
		t.Fatal("expected a non-zero exit code for a non-200 echo response")
	}
	if !strings.Contains(stderr, "status 401") {
		t.Errorf("stderr should mention the status, got %q", stderr)
	}
}

func TestEchoClientCommandUnreachable(t *testing.T) {
	deps, _ := defaultDeps(t)
	_, _, code := runCLIWith(&deps, "echo-client", "--url", "http://127.0.0.1:1")
	if code == exitCodeOK {
		t.Fatal("expected a non-zero exit code for an unreachable target")
	}
}

func TestDefaultServerURL(t *testing.T) {
	cfg := config.Defaults()
	app := &App{config: cfg}

	// 0.0.0.0 is mapped to localhost.
	if got := defaultServerURL(app); got != "http://localhost:8080" {
		t.Errorf("defaultServerURL = %q, want http://localhost:8080", got)
	}

	cfg.Server.Addr = "127.0.0.1"
	if got := defaultServerURL(app); got != "http://127.0.0.1:8080" {
		t.Errorf("defaultServerURL = %q, want http://127.0.0.1:8080", got)
	}
}
