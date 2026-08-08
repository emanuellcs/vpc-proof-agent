package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// errorResponse is the uniform JSON error body returned by the API. It
// carries the request ID so failures can be correlated with request logs.
type errorResponse struct {
	// Error is a safe, user-facing message.
	Error string `json:"error"`
	// RequestID correlates the response with the server-side logs.
	RequestID string `json:"request_id"`
	// Timestamp is when the error was generated.
	Timestamp time.Time `json:"timestamp"`
}

// requestIDContextKey carries the request ID through the context.
type requestIDContextKey struct{}

// requestIDFrom returns the request ID stored in the context, or an empty
// string when absent.
func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a safe JSON error containing the request ID and a
// timestamp. Internal details are never leaked to the client.
func writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	writeJSON(w, status, errorResponse{
		Error:     message,
		RequestID: requestIDFrom(r.Context()),
		Timestamp: time.Now().UTC(),
	})
}
