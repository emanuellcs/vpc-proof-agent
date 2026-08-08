package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
)

// appContextKey is the context key carrying the initialized application.
type appContextKey struct{}

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

// withAppContext stores the application container in the command context.
func withAppContext(cmd *cobra.Command, app *App) {
	ctx := context.WithValue(cmd.Context(), appContextKey{}, app)
	cmd.SetContext(ctx)
}

// appFrom returns the initialized application, or nil when unavailable.
func appFrom(cmd *cobra.Command) *App {
	app, _ := cmd.Context().Value(appContextKey{}).(*App)
	return app
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
