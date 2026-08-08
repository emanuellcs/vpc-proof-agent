package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/emanuellcs/vpc-proof-agent/internal/api"
	"github.com/emanuellcs/vpc-proof-agent/internal/api/cache"
	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

// newServeCommand starts the public REST API server.
func newServeCommand() *cobra.Command {
	var addr string
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the REST API server",
		Long: `Start the public REST API server that exposes the vpc-proof endpoints:
health and readiness checks, instance info, probe results, evidence reports,
and the reachability echo endpoint.`,
		Example: `  vpc-proof serve
  vpc-proof serve --addr 0.0.0.0 --port 9090`,
		GroupID: "administration",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd)
			if app == nil {
				return fmt.Errorf("serve: application is not initialized")
			}

			cfg := app.config
			if flags := cmd.Flags(); flags.Changed("addr") {
				cfg.Server.Addr = addr
			}
			if flags := cmd.Flags(); flags.Changed("port") {
				cfg.Server.Port = port
			}

			probeCache := cache.New(cfg.Cache.ProbeTTL.Value())
			metrics := observability.NewMetrics()

			server, err := api.New(api.Options{
				Config:   cfg,
				Logger:   app.logger,
				Metadata: app.metadata,
				Runner:   app.runner,
				Engine:   app.diagnostics,
				Cache:    probeCache,
				History:  app.history,
				Metrics:  metrics,
			})
			if err != nil {
				return fmt.Errorf("serve: %w", err)
			}

			if app.logger != nil {
				tlsEnabled := cfg.Server.TLSCertFile != "" && cfg.Server.TLSKeyFile != ""
				app.logger.Info("starting http server",
					observability.Component("cli"),
					observability.Str("addr", fmt.Sprintf("%s:%d", cfg.Server.Addr, cfg.Server.Port)),
					observability.Bool("tls_enabled", tlsEnabled),
					observability.Bool("auth_enabled", cfg.Auth.Enabled),
					observability.Int("rate_limit_per_minute", cfg.RateLimit.RequestsPerMinute),
					observability.Int("rate_limit_burst", cfg.RateLimit.Burst),
				)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			if app.history != nil {
				app.history.Start(ctx)
			}

			return runServer(ctx, server, cfg.Server.ShutdownTimeout.Value(), app.logger)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "", "listen address (overrides server.addr)")
	cmd.Flags().IntVar(&port, "port", 0, "listen port (overrides server.port)")

	return cmd
}

// runServer serves until ctx is canceled, then gracefully shuts down within
// the given timeout. It is separated from the signal wiring so the shutdown
// sequence can be unit-tested.
func runServer(ctx context.Context, server *api.Server, shutdownTimeout time.Duration, logger *observability.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		if logger != nil {
			logger.Info("http server stopped gracefully", observability.Component("cli"))
		}
		return nil
	}
}
