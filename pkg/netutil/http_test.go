package netutil

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// flakyRoundTripper fails the first `failures` requests, then delegates to
// the wrapped transport.
type flakyRoundTripper struct {
	failures int
	failErr  error
	attempts *int
	inner    http.RoundTripper
}

func (f *flakyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if *f.attempts < f.failures {
		*f.attempts++
		return nil, f.failErr
	}
	return f.inner.RoundTrip(req)
}

func newTestHTTPClient() *http.Client {
	return &http.Client{Timeout: 2 * time.Second}
}

func TestHTTPGetSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("  203.0.113.7\n"))
	}))
	t.Cleanup(server.Close)

	body, status, err := HTTPGet(context.Background(), newTestHTTPClient(), server.URL, 0)
	if err != nil {
		t.Fatalf("HTTPGet: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if body != "203.0.113.7" {
		t.Errorf("body = %q, want trimmed 203.0.113.7", body)
	}
}

func TestHTTPGetNon2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	_, status, err := HTTPGet(context.Background(), newTestHTTPClient(), server.URL, 3)
	if err == nil {
		t.Fatal("expected error for 503, got nil")
	}
	if status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", status)
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error should mention the status, got %v", err)
	}
}

func TestHTTPGetRetriesTransientErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	attempts := 0
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &flakyRoundTripper{
			failures: 2,
			failErr:  errors.New("transient network failure"),
			attempts: &attempts,
			inner:    http.DefaultTransport,
		},
	}

	body, status, err := HTTPGet(context.Background(), client, server.URL, 2)
	if err != nil {
		t.Fatalf("HTTPGet with retries: %v", err)
	}
	if status != http.StatusOK || body != "ok" {
		t.Errorf("got body=%q status=%d, want ok/200", body, status)
	}
	if attempts != 2 {
		t.Errorf("expected 2 transient failures, got %d", attempts)
	}
}

func TestHTTPGetExhaustsRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	attempts := 0
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &flakyRoundTripper{
			failures: 5,
			failErr:  errors.New("always failing"),
			attempts: &attempts,
			inner:    http.DefaultTransport,
		},
	}

	_, _, err := HTTPGet(context.Background(), client, server.URL, 2)
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if !strings.Contains(err.Error(), "3 attempts") {
		t.Errorf("error should mention the attempt count, got %v", err)
	}
}

func TestHTTPGetContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := HTTPGet(ctx, newTestHTTPClient(), server.URL, 0)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestHTTPGetBodyCapped(t *testing.T) {
	big := strings.Repeat("x", maxBodyBytes*2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(big))
	}))
	t.Cleanup(server.Close)

	body, _, err := HTTPGet(context.Background(), newTestHTTPClient(), server.URL, 0)
	if err != nil {
		t.Fatalf("HTTPGet: %v", err)
	}
	if len(body) > maxBodyBytes+1 {
		t.Errorf("body length = %d, want capped at %d", len(body), maxBodyBytes)
	}
}
