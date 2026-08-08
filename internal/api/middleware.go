package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

// middleware wraps an http.Handler, composing the request pipeline.
type middleware func(http.Handler) http.Handler

// requestIDHeader is the response header carrying the request ID.
const requestIDHeader = "X-Request-ID"

// generateRequestID returns a random 32-character hexadecimal request ID
// derived from crypto/rand, guaranteeing uniqueness and unpredictability.
func generateRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// withRequestID assigns a unique ID to every request, stores it in the
// context, and reflects it in the X-Request-ID response header.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := generateRequestID()
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, id)
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// recoverMiddleware catches panics, logs the stack trace, and returns a safe
// 500 response instead of crashing the process or leaking internal details.
func recoverMiddleware(logger *observability.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if logger != nil {
						logger.Error("panic recovered",
							observability.Component("api"),
							observability.Str("request_id", requestIDFrom(r.Context())),
							observability.Any("panic", recovered),
							observability.Str("stack", string(debug.Stack())),
						)
					}
					writeError(w, r, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// securityHeaders injects hardening headers into every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// statusWriter records the response status code and byte count for logging
// and metrics.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

// WriteHeader records the status before delegating.
func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write records the status and byte count before delegating.
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// requestLogging records every request in the metrics registry and the
// structured log, including method, path, status, duration, and request ID.
func requestLogging(logger *observability.Logger, metrics *observability.Metrics) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			writer := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(writer, r)

			status := writer.status
			if status == 0 {
				status = http.StatusOK
			}
			duration := time.Since(start)

			if metrics != nil {
				metrics.IncRequest(r.Method, r.URL.Path, strconv.Itoa(status))
				metrics.ObserveRequestDuration(r.Method, r.URL.Path, duration)
			}
			if logger != nil {
				logger.Info("http request",
					observability.Component("api"),
					observability.Str("request_id", requestIDFrom(r.Context())),
					observability.Str("method", r.Method),
					observability.Str("path", r.URL.Path),
					observability.Int("status", status),
					observability.Duration("duration", duration),
				)
			}
		})
	}
}

// authMiddleware enforces bearer-token authentication when enabled,
// bypassing the configured public paths.
func authMiddleware(auth *config.AuthConfig) middleware {
	return func(next http.Handler) http.Handler {
		if !auth.Enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path, auth.PublicPaths) {
				next.ServeHTTP(w, r)
				return
			}
			if !constantTimeEqual(bearerToken(r), auth.Token) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="vpc-proof"`)
				writeError(w, r, http.StatusUnauthorized, "unauthorized: missing or invalid bearer token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isPublicPath reports whether a path is exempt from authentication.
func isPublicPath(path string, publicPaths []string) bool {
	for _, candidate := range publicPaths {
		if candidate == path {
			return true
		}
	}
	return false
}

// bearerToken extracts the token from an Authorization: Bearer header.
func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}

// constantTimeEqual compares two strings in constant time to avoid timing
// side channels during token validation.
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
