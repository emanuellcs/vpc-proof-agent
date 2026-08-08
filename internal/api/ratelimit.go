package api

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
)

// Rate limiter constants.
const (
	// limiterCleanupInterval is how often idle client limiters are pruned.
	limiterCleanupInterval = 10 * time.Minute
	// limiterIdleTimeout is how long a client limiter may stay idle before
	// being removed, bounding memory growth.
	limiterIdleTimeout = 10 * time.Minute
)

// rateLimiter provides per-client-IP token buckets built on x/time/rate.
type rateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*clientLimiter
	rate     rate.Limit
	burst    int
	now      func() time.Time
}

// clientLimiter pairs a token bucket with its last-seen timestamp.
type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// newRateLimiter builds a limiter from the rate-limit configuration.
func newRateLimiter(cfg *config.RateLimitConfig) *rateLimiter {
	return &rateLimiter{
		limiters: map[string]*clientLimiter{},
		rate:     rate.Limit(float64(cfg.RequestsPerMinute) / 60.0),
		burst:    cfg.Burst,
		now:      time.Now,
	}
}

// Allow reports whether a request from ip may proceed. When it may not, the
// returned duration is a recommended retry wait.
func (l *rateLimiter) Allow(ip string) (ok bool, retryAfter time.Duration) {
	limiter := l.limiterFor(ip)
	if limiter.Allow() {
		return true, 0
	}
	return false, l.retryAfter()
}

// limiterFor returns the token bucket for an IP, creating it on first use.
func (l *rateLimiter) limiterFor(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.limiters[ip]
	if !ok {
		entry = &clientLimiter{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.limiters[ip] = entry
	}
	entry.lastSeen = l.now()
	return entry.limiter
}

// retryAfter approximates the wait until the next token is available.
func (l *rateLimiter) retryAfter() time.Duration {
	if l.rate <= 0 {
		return time.Second
	}
	return time.Duration(float64(time.Second) / float64(l.rate))
}

// RunCleanup prunes idle client limiters until ctx is canceled. It is
// intended to run in a goroutine owned by the server and must be stopped via
// the server's Shutdown so it never leaks.
func (l *rateLimiter) RunCleanup(ctx context.Context) {
	ticker := time.NewTicker(limiterCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.pruneIdle()
		}
	}
}

// pruneIdle removes limiters that have been idle longer than the timeout.
func (l *rateLimiter) pruneIdle() {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := l.now().Add(-limiterIdleTimeout)
	for ip, entry := range l.limiters {
		if entry.lastSeen.Before(cutoff) {
			delete(l.limiters, ip)
		}
	}
}

// size returns the number of tracked client limiters (used in tests).
func (l *rateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.limiters)
}

// rateLimitMiddleware rejects clients that exceed their token budget with
// 429 and a Retry-After header. Infrastructure endpoints are exempt so load
// balancers and probes are never throttled.
func rateLimitMiddleware(limiter *rateLimiter) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isInfraPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			ok, retryAfter := limiter.Allow(clientIP(r))
			if !ok {
				seconds := int(math.Ceil(retryAfter.Seconds()))
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				writeError(w, r, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the client IP from RemoteAddr, dropping the port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// clientIPFromRequest resolves the best-effort client IP, preferring proxy
// headers (X-Forwarded-For, X-Real-IP) before falling back to the direct
// connection address.
func clientIPFromRequest(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if first := strings.TrimSpace(strings.Split(forwarded, ",")[0]); first != "" {
			return first
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	return clientIP(r)
}

// isInfraPath reports whether a path is exempt from rate limiting.
func isInfraPath(path string) bool {
	switch path {
	case "/healthz", "/readyz", "/metrics":
		return true
	default:
		return false
	}
}
