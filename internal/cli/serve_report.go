package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newReportCommand generates evidence reports from probe results.
func newReportCommand() *cobra.Command {
	var format string
	var output string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate an evidence report",
		Long: `Generate a comprehensive evidence report from probe results in JSON,
Markdown, or plain text. This command is a placeholder in the current commit
and will be fully implemented in a later commit.`,
		Example: `  vpc-proof report
  vpc-proof report --format json
  vpc-proof report --format markdown --output report.md`,
		GroupID: "diagnostics",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch format {
			case "json", "markdown", "text":
			default:
				return fmt.Errorf("invalid report format %q (expected json, markdown, or text)", format)
			}

			dest := output
			if dest == "" || dest == "-" {
				dest = "stdout"
			}
			cmd.Printf("[stub] report: format=%s output=%s (not yet implemented)\n", format, dest)
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "report format: json, markdown, or text")
	cmd.Flags().StringVar(&output, "output", "-", "output destination: a file path, or - for stdout")

	return cmd
}

// newServeCommand starts the public REST API server.
func newServeCommand() *cobra.Command {
	var addr string
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the REST API server",
		Long: `Start the public REST API server that exposes the vpc-proof endpoints.
This command is a placeholder in the current commit and will be fully
implemented in a later commit.`,
		Example: `  vpc-proof serve
  vpc-proof serve --addr 0.0.0.0 --port 9090`,
		GroupID: "administration",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := configFrom(cmd)
			if cfg == nil {
				return fmt.Errorf("serve: configuration is not available")
			}

			effectiveAddr := cfg.Server.Addr
			effectivePort := cfg.Server.Port
			flags := cmd.Flags()
			if flags.Changed("addr") {
				effectiveAddr = addr
			}
			if flags.Changed("port") {
				effectivePort = port
			}

			cmd.Printf("[stub] serve: listening on %s:%d (not yet implemented)\n", effectiveAddr, effectivePort)
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "", "listen address (overrides server.addr)")
	cmd.Flags().IntVar(&port, "port", 0, "listen port (overrides server.port)")

	return cmd
}
