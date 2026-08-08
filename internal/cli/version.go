package cli

import (
	"runtime"

	"github.com/spf13/cobra"

	"github.com/emanuellcs/vpc-proof-agent/internal/buildinfo"
)

// newVersionCommand reports build and runtime information.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Long: `Print the semantic version, git commit, build date, Go runtime version,
and target platform of the vpc-proof binary.`,
		Example: "  vpc-proof version",
		GroupID: "info",
		Args:    cobra.NoArgs,
		// version does not depend on the configuration, so it replaces the
		// inherited bootstrap hook.
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Printf("vpc-proof\n")
			cmd.Printf("  version:    %s\n", buildinfo.Version)
			cmd.Printf("  commit:     %s\n", buildinfo.Commit)
			cmd.Printf("  build date: %s\n", buildinfo.BuildDate)
			cmd.Printf("  go version: %s\n", runtime.Version())
			cmd.Printf("  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}
