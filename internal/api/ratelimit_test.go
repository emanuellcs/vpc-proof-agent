package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
)

func TestRateLimiterAllow(t *testing.T) {
	limiter := newRateLimiter(&config.RateLimitConfig{RequestsPerMinute: 2, Burst: 2})

	for i := range 2 {
		ok, retryAfter := limiter.Allow("10.0.0.1")
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
		if retryAfter != 0 {
			t.Errorf("request %d retryAfter = %s, want 0", i+1, retryAfter)
		}
	}

	ok, retryAfter := limiter.Allow("10.0.0.1")
	if ok {
		t.Fatal("third request should be rate limited")
	}
	if retryAfter <= 0 {
		t.Errorf("retryAfter = %s, want positive", retryAfter)
	}
}

func TestRateLimiterPerIPIsolation(t *testing.T) {
	limiter := newRateLimiter(&config.RateLimitConfig{RequestsPerMinute: 1, Burst: 1})

	if ok, _ := limiter.Allow("10.0.0.1"); !ok {
		t.Fatal("10.0.0.1 first request should be allowed")
	}
	if ok, _ := limiter.Allow("10.0.0.2"); !ok {
		t.Fatal("different IP should have its own budget")
	}
	if ok, _ := limiter.Allow("10.0.0.1"); ok {
		t.Fatal("10.0.0.1 second request should be rate limited")
	}
	if size := limiter.size(); size != 2 {
		t.Errorf("tracked IPs = %d, want 2", size)
	}
}

func TestRateLimiterPruneIdle(t *testing.T) {
	base := time.Now()
	limiter := newRateLimiter(&config.RateLimitConfig{RequestsPerMinute: 60, Burst: 1})
	limiter.now = func() time.Time { return base }

	limiter.Allow("10.0.0.1")
	limiter.Allow("10.0.0.2")
	if limiter.size() != 2 {
		t.Fatalf("expected 2 tracked IPs before pruning, got %d", limiter.size())
	}

	// Advance past the idle timeout and prune.
	limiter.now = func() time.Time { return base.Add(limiterIdleTimeout + time.Minute) }
	limiter.pruneIdle()
	if limiter.size() != 0 {
		t.Errorf("idle limiters not pruned: %d remaining", limiter.size())
	}
}

func TestRateLimiterRunCleanupStops(t *testing.T) {
	limiter := newRateLimiter(&config.RateLimitConfig{RequestsPerMinute: 60, Burst: 1})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		limiter.RunCleanup(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup goroutine did not stop after cancel")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	_, _, server := newTestServer(t, func(cfg *config.Config) {
		cfg.RateLimit.RequestsPerMinute = 2
		cfg.RateLimit.Burst = 2
	})

	for i := range 2 {
		resp := get(t, server.URL+"/api/v1/echo")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i+1, resp.StatusCode)
		}
	}

	resp := get(t, server.URL+"/api/v1/echo")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limited request status = %d, want 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("missing Retry-After header on 429")
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode 429 body: %v", err)
	}
	if body["request_id"] == "" {
		t.Errorf("429 body should carry the request id: %v", body)
	}

	// Infrastructure endpoints are exempt from rate limiting.
	resp = get(t, server.URL+"/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200 (rate limit exempt)", resp.StatusCode)
	}
}

func TestRateLimiterRetryAfterZeroRate(t *testing.T) {
	limiter := &rateLimiter{rate: 0, burst: 1, limiters: map[string]*clientLimiter{}}
	if retryAfter := limiter.retryAfter(); retryAfter != time.Second {
		t.Errorf("retryAfter = %s, want 1s fallback", retryAfter)
	}
}

func TestClientIPWithoutPort(t *testing.T) {
	req := newRequest("no-port")
	if ip := clientIP(req); ip != "no-port" {
		t.Errorf("clientIP = %q, want the raw RemoteAddr", ip)
	}
}

func TestClientIP(t *testing.T) {
	req := newRequest("10.0.0.1:1234")
	if ip := clientIP(req); ip != "10.0.0.1" {
		t.Errorf("clientIP = %q, want 10.0.0.1", ip)
	}
}

func TestClientIPFromRequest(t *testing.T) {
	req := newRequest("10.0.0.1:1234")
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.0.0.1")
	if ip := clientIPFromRequest(req); ip != "198.51.100.9" {
		t.Errorf("X-Forwarded-For = %q, want first entry", ip)
	}

	req = newRequest("10.0.0.1:1234")
	req.Header.Set("X-Real-IP", "198.51.100.10")
	if ip := clientIPFromRequest(req); ip != "198.51.100.10" {
		t.Errorf("X-Real-IP = %q, want 198.51.100.10", ip)
	}

	req = newRequest("10.0.0.1:1234")
	if ip := clientIPFromRequest(req); ip != "10.0.0.1" {
		t.Errorf("fallback = %q, want RemoteAddr host", ip)
	}
}
