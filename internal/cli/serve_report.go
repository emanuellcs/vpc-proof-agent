package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

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
