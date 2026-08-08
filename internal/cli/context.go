package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

// appContextKey is the context key carrying the loaded application state.
type appContextKey struct{}

// appContext holds the dependencies injected into the command context by the
// root command's PersistentPreRunE.
type appContext struct {
	config *config.Config
	logger *observability.Logger
}

// validationContextKey is the context key used by validate-config to carry
// the load results without aborting on failure.
type validationContextKey struct{}

// validationResult carries the outcome of loading and validating the
// configuration for the validate-config command.
type validationResult struct {
	config  *config.Config
	errs    []error
	loadErr error
}

// withAppContext stores cfg and logger in the command context.
func withAppContext(cmd *cobra.Command, cfg *config.Config, logger *observability.Logger) {
	ctx := context.WithValue(cmd.Context(), appContextKey{}, &appContext{config: cfg, logger: logger})
	cmd.SetContext(ctx)
}

// configFrom returns the loaded configuration, or nil when unavailable.
func configFrom(cmd *cobra.Command) *config.Config {
	app, _ := cmd.Context().Value(appContextKey{}).(*appContext)
	if app == nil {
		return nil
	}
	return app.config
}

// loggerFrom returns the initialized logger, or nil when unavailable.
func loggerFrom(cmd *cobra.Command) *observability.Logger {
	app, _ := cmd.Context().Value(appContextKey{}).(*appContext)
	if app == nil {
		return nil
	}
	return app.logger
}

// withValidationResult stores the result of a load/validate pass.
func withValidationResult(cmd *cobra.Command, result validationResult) {
	ctx := context.WithValue(cmd.Context(), validationContextKey{}, result)
	cmd.SetContext(ctx)
}

// validationResultFrom returns the stored validation result.
func validationResultFrom(cmd *cobra.Command) (validationResult, bool) {
	result, ok := cmd.Context().Value(validationContextKey{}).(validationResult)
	return result, ok
}
