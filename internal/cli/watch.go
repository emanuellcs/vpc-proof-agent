package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

// newWatchCommand continuously runs the probe suite.
func newWatchCommand() *cobra.Command {
	var interval time.Duration
	var noClear bool

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Continuously run the probe suite",
		Long: `Run the probe suite on a loop, refreshing the terminal with the latest
status. Stops cleanly on SIGINT or SIGTERM.`,
		Example: "  vpc-proof watch\n  vpc-proof watch --interval 15s",
		GroupID: "diagnostics",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd)
			if app == nil {
				return fmt.Errorf("watch: application is not initialized")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runWatch(ctx, app, interval, !noClear, cmd.OutOrStdout())
		},
	}

	cmd.Flags().DurationVar(&interval, "interval", 30*time.Second, "interval between runs")
	cmd.Flags().BoolVar(&noClear, "no-clear", false, "do not clear the screen between runs")

	return cmd
}

// runWatch loops until ctx is canceled, printing the latest status on each
// interval. It is separated from the signal wiring so it can be unit-tested.
func runWatch(ctx context.Context, app *App, interval time.Duration, clearScreen bool, out io.Writer) error {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		report := app.RunProbes(ctx)
		printWatchStatus(out, report, clearScreen)

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// printWatchStatus clears the terminal (unless disabled) and prints the probe
// summary.
func printWatchStatus(out io.Writer, report probe.Report, clearScreen bool) {
	if clearScreen {
		fmt.Fprint(out, "\x1b[2J\x1b[H")
	}
	fmt.Fprintln(out, "=== vpc-proof watch ===")
	printProbeSummary(out, report)
}
