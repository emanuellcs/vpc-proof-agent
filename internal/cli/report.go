package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/emanuellcs/vpc-proof-agent/internal/report"
)

// newReportCommand generates evidence reports from probe results.
func newReportCommand() *cobra.Command {
	var formatName string
	var output string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate an evidence report",
		Long: `Generate a comprehensive evidence report from probe results in JSON,
Markdown, or plain text.`,
		Example: `  vpc-proof report
  vpc-proof report --format json
  vpc-proof report --format markdown --output report.md`,
		GroupID: "diagnostics",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := report.ParseFormat(formatName)
			if err != nil {
				return err
			}

			app := appFrom(cmd)
			if app == nil {
				return fmt.Errorf("report: application is not initialized")
			}

			probeReport := app.RunProbes(cmd.Context())
			hints := app.Diagnose(probeReport)
			agent := report.AgentInfoFromRuntime()
			data := report.Build(probeReport, hints, &agent)

			engine, err := report.New()
			if err != nil {
				return err
			}

			if output == "" || output == "-" {
				return engine.Write(cmd.OutOrStdout(), &data, format)
			}

			if err := engine.WriteFile(output, &data, format); err != nil {
				return err
			}
			abs, absErr := filepath.Abs(output)
			if absErr != nil {
				abs = output
			}
			cmd.Printf("report written to %s\n", abs)
			return nil
		},
	}

	cmd.Flags().StringVar(&formatName, "format", "text", "report format: json, markdown, or text")
	cmd.Flags().StringVar(&output, "output", "-", "output destination: a file path, or - for stdout")

	return cmd
}
