package cli

import (
	"github.com/spf13/cobra"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

// newStatusCommand shows a quick summary of the instance.
func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show a quick summary of the instance",
		Long: `Show a quick summary of the EC2 instance, including its metadata and a
high-level network overview. This command is a placeholder in the current
commit and will be fully implemented in a later commit.`,
		Example: "  vpc-proof status",
		GroupID: "diagnostics",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if log := loggerFrom(cmd); log != nil {
				log.Info("status command invoked", observability.Component("cli"))
			}
			cmd.Println("[stub] status: not yet implemented")
			return nil
		},
	}
}

// newCheckCommand runs the full probe suite.
func newCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run the full network probe suite",
		Long: `Run every network and metadata probe and report the combined results.
This command is a placeholder in the current commit and will be fully
implemented in a later commit.`,
		Example: "  vpc-proof check",
		GroupID: "diagnostics",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if log := loggerFrom(cmd); log != nil {
				log.Info("check command invoked", observability.Component("cli"))
			}
			cmd.Println("[stub] check: not yet implemented")
			return nil
		},
	}
}

// newDiagnoseCommand runs probes and outputs troubleshooting hints.
func newDiagnoseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose",
		Short: "Run probes and output troubleshooting hints",
		Long: `Run the probe suite and translate any failure into actionable AWS
troubleshooting hints. This command is a placeholder in the current commit
and will be fully implemented in a later commit.`,
		Example: "  vpc-proof diagnose",
		GroupID: "diagnostics",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if log := loggerFrom(cmd); log != nil {
				log.Info("diagnose command invoked", observability.Component("cli"))
			}
			cmd.Println("[stub] diagnose: not yet implemented")
			return nil
		},
	}
}
