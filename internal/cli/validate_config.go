package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newValidateConfigCommand loads and validates the configuration without
// aborting, printing a success message or the detailed list of errors.
func newValidateConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-config",
		Short: "Validate the configuration",
		Long: `Load the configuration from the default sources (command-line flags,
environment variables, and the YAML config file) and report whether it is
valid. Prints a success message or a detailed list of validation errors.`,
		Example: "  vpc-proof validate-config\n  vpc-proof validate-config --config config.example.yaml",
		GroupID: "administration",
		Args:    cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, errs, err := loadConfiguration(cmd)
			withValidationResult(cmd, validationResult{config: cfg, errs: errs, loadErr: err})
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, ok := validationResultFrom(cmd)
			if !ok {
				return nil
			}
			if result.loadErr != nil {
				return fmt.Errorf("configuration: %w", result.loadErr)
			}
			if len(result.errs) > 0 {
				return validationErrors(result.errs)
			}
			cmd.Println("configuration is valid")
			return nil
		},
	}
}
