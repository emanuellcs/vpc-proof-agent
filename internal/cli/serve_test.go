package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/api"
	"github.com/emanuellcs/vpc-proof-agent/internal/api/cache"
	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/diagnostic"
	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

// testAPIServer builds a minimal API server for lifecycle tests.
func testAPIServer(t *testing.T, cfg *config.Config) *api.Server {
	t.Helper()

	logger, err := observability.New("info", "json", io.Discard)
	if err != nil {
		t.Fatalf("observability.New: %v", err)
	}
	t.Cleanup(func() { _ = logger.Sync() })

	server, err := api.New(api.Options{
		Config: cfg,
		Logger: logger,
		Runner: probe.NewRunner(nil),
		Engine: diagnostic.New(),
		Cache:  cache.New(time.Minute),
	})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	return server
}

// freePort reserves then releases a TCP port so the server can bind to it.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// waitForListening polls until the port accepts connections.
func waitForListening(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not start listening")
}

func TestRunServerGracefulShutdown(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.Addr = "127.0.0.1"
	cfg.Server.Port = freePort(t)

	server := testAPIServer(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runServer(ctx, server, 2*time.Second, nil)
	}()

	waitForListening(t, cfg.Server.Port)

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServer returned error on graceful shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServer did not return after context cancellation")
	}
}

func TestRunServerReturnsServeError(t *testing.T) {
	// Occupy a port so ListenAndServe fails immediately.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	cfg := config.Defaults()
	cfg.Server.Addr = "127.0.0.1"
	cfg.Server.Port = port

	server := testAPIServer(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := runServer(ctx, server, time.Second, nil); err == nil {
		t.Fatal("expected an error when the address is already in use, got nil")
	}
}

func TestServeCommandFailsOnOccupiedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	deps, _ := defaultDeps(t)

	stdout, stderr, code := runCLIWith(deps, "serve", "--addr", "127.0.0.1", "--port", fmt.Sprintf("%d", port))
	if code == exitCodeOK {
		t.Fatalf("expected a non-zero exit code when the port is occupied, got 0")
	}
	if !strings.Contains(stderr, "address already in use") {
		t.Errorf("stderr should mention the bind failure, got %q", stderr)
	}
	if strings.Contains(stdout, "stub") {
		t.Error("serve should no longer be a stub")
	}
}
