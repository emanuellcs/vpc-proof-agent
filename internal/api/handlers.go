package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/api/cache"
	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/diagnostic"
	"github.com/emanuellcs/vpc-proof-agent/internal/history"
	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
	"github.com/emanuellcs/vpc-proof-agent/internal/report"
	"github.com/emanuellcs/vpc-proof-agent/pkg/metadata"
)

// forceRefreshHeader, when set to "true", bypasses the probe cache and forces
// a fresh probe execution.
const forceRefreshHeader = "X-Force-Refresh"

// redactedToken is the placeholder replacing sensitive configuration values.
const redactedToken = "[REDACTED]"

// knownRoutes maps each registered path to the methods it accepts, used to
// emit 405 Method Not Allowed from the catch-all handler.
var knownRoutes = map[string][]string{
	"/healthz":             {http.MethodGet},
	"/readyz":              {http.MethodGet},
	"/metrics":             {http.MethodGet},
	"/api/v1/info":         {http.MethodGet},
	"/api/v1/status":       {http.MethodGet},
	"/api/v1/network":      {http.MethodGet},
	"/api/v1/probe":        {http.MethodGet},
	"/api/v1/report":       {http.MethodGet},
	"/api/v1/echo":         {http.MethodGet},
	"/api/v1/history":      {http.MethodGet},
	"/api/v1/config":       {http.MethodGet},
	"/api/v1/openapi.json": {http.MethodGet},
}

// Handlers implements the HTTP endpoints. It is intentionally decoupled from
// the CLI container: dependencies are injected as plain fields.
type Handlers struct {
	config   *config.Config
	logger   *observability.Logger
	metadata metadata.Client
	runner   *probe.Runner
	engine   *diagnostic.Engine
	cache    *cache.Cache
	history  *history.Store
	metrics  *observability.Metrics

	// refreshMu serializes concurrent cache refreshes to prevent a stampede.
	refreshMu sync.Mutex
}

// routes registers every endpoint with the method-based ServeMux patterns.
func (h *Handlers) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.HandleFunc("GET /api/v1/info", h.info)
	mux.HandleFunc("GET /api/v1/status", h.status)
	mux.HandleFunc("GET /api/v1/network", h.network)
	mux.HandleFunc("GET /api/v1/probe", h.probe)
	mux.HandleFunc("GET /api/v1/report", h.report)
	mux.HandleFunc("GET /api/v1/echo", h.echo)
	mux.HandleFunc("GET /api/v1/history", h.historyHandler)
	mux.HandleFunc("GET /api/v1/config", h.configHandler)
	mux.HandleFunc("GET /api/v1/openapi.json", h.openAPIHandler)
	mux.HandleFunc("GET /metrics", h.metricsHandler)
	mux.HandleFunc("/", h.notFound)
	return mux
}

// notFound is the catch-all handler that returns a consistent JSON 404, or a
// 405 when the path is known but the method is not allowed.
func (h *Handlers) notFound(w http.ResponseWriter, r *http.Request) {
	if methods, ok := knownRoutes[r.URL.Path]; ok && !slices.Contains(methods, r.Method) {
		w.Header().Set("Allow", strings.Join(methods, ", "))
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeError(w, r, http.StatusNotFound, "not found")
}

// health reports liveness.
func (h *Handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ready reports readiness, verifying the essential dependencies are wired.
func (h *Handlers) ready(w http.ResponseWriter, r *http.Request) {
	if h.runner == nil || h.cache == nil {
		writeError(w, r, http.StatusServiceUnavailable, "not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// info returns the agent build information and basic instance metadata.
func (h *Handlers) info(w http.ResponseWriter, r *http.Request) {
	agent := report.AgentInfoFromRuntime()
	writeJSON(w, http.StatusOK, map[string]any{
		"agent":    agent,
		"instance": h.fetchInstance(r.Context()),
	})
}

// status returns the aggregated probe status and counts.
func (h *Handlers) status(w http.ResponseWriter, r *http.Request) {
	data := h.cachedData(r.Context())
	writeJSON(w, http.StatusOK, data.Summary)
}

// network returns the derived network summary.
func (h *Handlers) network(w http.ResponseWriter, r *http.Request) {
	data := h.cachedData(r.Context())
	writeJSON(w, http.StatusOK, data.Network)
}

// probe returns the full probe report, honoring a forced refresh.
func (h *Handlers) probe(w http.ResponseWriter, r *http.Request) {
	data := h.resolvedData(r)
	writeJSON(w, http.StatusOK, data.Probes)
}

// report renders the full evidence report in the requested format,
// defaulting to JSON.
func (h *Handlers) report(w http.ResponseWriter, r *http.Request) {
	data := h.resolvedData(r)

	format, err := report.ParseFormat(r.URL.Query().Get("format"))
	if err != nil {
		format = report.FormatJSON
	}

	engine, err := report.New()
	if err != nil {
		h.logError(r, "build report engine", err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}

	rendered, err := engine.Render(&data, format)
	if err != nil {
		h.logError(r, "render report", err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", contentType(format))
	// #nosec G705 -- the report is rendered by the agent's own embedded
	// templates and served as markdown/plain text, never as HTML.
	_, _ = w.Write(rendered)
}

// echo reflects the requester's IP, User-Agent, and request time, proving
// external reachability.
func (h *Handlers) echo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ip":          clientIPFromRequest(r),
		"user_agent":  r.UserAgent(),
		"received_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// metricsHandler exposes the Prometheus-compatible text metrics.
func (h *Handlers) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if err := h.metrics.WritePrometheus(w); err != nil {
		h.logError(r, "write metrics", err)
	}
}

// historyHandler returns the past probe run summaries.
func (h *Handlers) historyHandler(w http.ResponseWriter, _ *http.Request) {
	if h.history == nil {
		writeJSON(w, http.StatusOK, []history.Entry{})
		return
	}
	writeJSON(w, http.StatusOK, h.history.List())
}

// configHandler returns the loaded configuration with sensitive fields
// redacted.
func (h *Handlers) configHandler(w http.ResponseWriter, r *http.Request) {
	if h.config == nil {
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, sanitizedConfig(h.config))
}

// openAPIHandler serves the embedded OpenAPI 3.0 specification.
func (h *Handlers) openAPIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(openAPIDocument); err != nil {
		h.logError(r, "write openapi document", err)
	}
}

// sanitizedConfig returns a copy of the configuration with sensitive values
// replaced by a redaction placeholder.
func sanitizedConfig(cfg *config.Config) *config.Config {
	sanitized := *cfg
	sanitized.Auth.Token = redactedToken
	return &sanitized
}

// fetchInstance gathers instance metadata, degrading gracefully when the
// metadata service is unavailable.
func (h *Handlers) fetchInstance(ctx context.Context) report.Instance {
	instance := report.Instance{}
	if h.metadata == nil {
		return instance
	}
	if id, err := h.metadata.InstanceID(ctx); err == nil {
		instance.InstanceID = id
	}
	if az, err := h.metadata.AvailabilityZone(ctx); err == nil {
		instance.AvailabilityZone = az
	}
	if ip, err := h.metadata.PrivateIP(ctx); err == nil {
		instance.PrivateIP = ip
	}
	if ip, err := h.metadata.PublicIP(ctx); err == nil {
		instance.PublicIP = ip
	}
	return instance
}

// cachedData returns the cached report, refreshing it on a miss.
func (h *Handlers) cachedData(ctx context.Context) report.Data {
	if data, ok := h.cache.Get(); ok {
		return data
	}
	return h.refresh(ctx, false)
}

// resolvedData returns the report data, honoring the X-Force-Refresh header.
func (h *Handlers) resolvedData(r *http.Request) report.Data {
	if strings.EqualFold(r.Header.Get(forceRefreshHeader), "true") {
		return h.refresh(r.Context(), true)
	}
	return h.cachedData(r.Context())
}

// refresh runs the probe suite, diagnoses the results, updates the metrics,
// and caches the assembled report. When force is false, a valid cached entry
// short-circuits the run (preventing stampedes); force bypasses the cache.
func (h *Handlers) refresh(ctx context.Context, force bool) report.Data {
	h.refreshMu.Lock()
	defer h.refreshMu.Unlock()

	if !force {
		if data, ok := h.cache.Get(); ok {
			return data
		}
	}

	probeReport := h.runner.Run(ctx)
	hints := h.engine.Analyze(probeReport)
	agent := report.AgentInfoFromRuntime()
	data := report.Build(probeReport, hints, &agent)

	if h.metrics != nil {
		for _, result := range probeReport.Results {
			h.metrics.SetProbeStatus(result.ID, result.Status.String())
		}
	}
	if h.history != nil {
		entry := history.FromReport(probeReport)
		h.history.Append(&entry)
	}

	h.cache.Put(&data)
	return data
}

// logError records an error through the structured logger when available.
func (h *Handlers) logError(r *http.Request, message string, err error) {
	if h.logger != nil {
		h.logger.Error(message,
			observability.Component("api"),
			observability.Str("request_id", requestIDFrom(r.Context())),
			observability.Error(err),
		)
	}
}

// contentType maps a report format to its HTTP content type.
func contentType(format report.Format) string {
	switch format {
	case report.FormatMarkdown:
		return "text/markdown; charset=utf-8"
	case report.FormatText:
		return "text/plain; charset=utf-8"
	default:
		return "application/json"
	}
}
