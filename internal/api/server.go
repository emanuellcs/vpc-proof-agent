// Package api implements the public, versioned REST API (v1) for the
// vpc-proof agent.
//
// It owns the HTTP server, routing, request handlers, and the middleware
// stack (request IDs, structured logging, recovery, security headers, token
// authentication, and rate limiting). Handlers delegate to the probe,
// diagnostic, and report packages and never access AWS credentials.
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/emanuellcs/vpc-proof-agent/internal/api/cache"
	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/diagnostic"
	"github.com/emanuellcs/vpc-proof-agent/internal/history"
	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
	"github.com/emanuellcs/vpc-proof-agent/pkg/metadata"
)

// Options carries the dependencies required to build the API server. It is
// assembled by the serve command from the application container, keeping the
// api package free of a dependency on internal/cli (avoiding an import
// cycle).
type Options struct {
	// Config is the validated application configuration.
	Config *config.Config
	// Logger is the structured logger (may be nil).
	Logger *observability.Logger
	// Metadata is the IMDSv2 client (may be nil).
	Metadata metadata.Client
	// Runner executes the probe suite.
	Runner *probe.Runner
	// Engine translates probe reports into troubleshooting hints.
	Engine *diagnostic.Engine
	// Cache stores the most recent probe report.
	Cache *cache.Cache
	// History tracks past probe run summaries (may be nil).
	History *history.Store
	// Metrics records HTTP and probe metrics (defaults to a fresh registry).
	Metrics *observability.Metrics
}

// Server is the vpc-proof HTTP server.
type Server struct {
	httpServer *http.Server
	handler    http.Handler
	logger     *observability.Logger

	rateLimiter *rateLimiter
	// cleanupCancel stops the rate-limiter cleanup goroutine on shutdown.
	cleanupCancel context.CancelFunc
	// tlsCertFile and tlsKeyFile, when both set, enable HTTPS serving.
	tlsCertFile string
	tlsKeyFile  string
}

// New builds a Server from the given options, wiring the routing table and
// the middleware chain. The rate-limiter cleanup goroutine is started here
// and stopped by Shutdown.
func New(opts Options) (*Server, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("api: config is required")
	}
	if opts.Runner == nil {
		return nil, fmt.Errorf("api: probe runner is required")
	}
	if opts.Engine == nil {
		return nil, fmt.Errorf("api: diagnostic engine is required")
	}
	if opts.Cache == nil {
		return nil, fmt.Errorf("api: probe cache is required")
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}

	handlers := &Handlers{
		config:   opts.Config,
		logger:   opts.Logger,
		metadata: opts.Metadata,
		runner:   opts.Runner,
		engine:   opts.Engine,
		cache:    opts.Cache,
		history:  opts.History,
		metrics:  opts.Metrics,
	}

	limiter := newRateLimiter(&opts.Config.RateLimit)

	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	go limiter.RunCleanup(cleanupCtx)

	root := http.Handler(handlers.routes())
	root = chain(root,
		withRequestID,
		recoverMiddleware(opts.Logger),
		securityHeaders,
		requestLogging(opts.Logger, opts.Metrics),
		rateLimitMiddleware(limiter),
		authMiddleware(&opts.Config.Auth),
	)

	addr := fmt.Sprintf("%s:%d", opts.Config.Server.Addr, opts.Config.Server.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      root,
		ReadTimeout:  opts.Config.Server.ReadTimeout.Value(),
		WriteTimeout: opts.Config.Server.WriteTimeout.Value(),
		IdleTimeout:  opts.Config.Server.IdleTimeout.Value(),
	}

	return &Server{
		httpServer:    httpServer,
		handler:       root,
		logger:        opts.Logger,
		rateLimiter:   limiter,
		cleanupCancel: cleanupCancel,
		tlsCertFile:   opts.Config.Server.TLSCertFile,
		tlsKeyFile:    opts.Config.Server.TLSKeyFile,
	}, nil
}

// Handler returns the fully wrapped HTTP handler, used directly by httptest
// servers in integration tests.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// ListenAndServe starts serving on the configured address, using HTTPS when
// both a TLS certificate and key are configured.
func (s *Server) ListenAndServe() error {
	if s.tlsCertFile != "" && s.tlsKeyFile != "" {
		return s.httpServer.ListenAndServeTLS(s.tlsCertFile, s.tlsKeyFile)
	}
	return s.httpServer.ListenAndServe()
}

// TLSCertFile returns the configured TLS certificate path, or an empty string
// when TLS is disabled.
func (s *Server) TLSCertFile() string {
	return s.tlsCertFile
}

// Shutdown gracefully stops the server, waiting for in-flight requests up to
// the deadline in ctx. It also stops the rate-limiter cleanup goroutine.
func (s *Server) Shutdown(ctx context.Context) error {
	s.cleanupCancel()
	return s.httpServer.Shutdown(ctx)
}

// chain composes middlewares so that the first entry is the outermost.
func chain(handler http.Handler, middlewares ...middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
