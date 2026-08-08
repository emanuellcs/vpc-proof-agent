package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

// newStatusCommand shows a quick summary of the instance.
func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show a quick summary of the instance",
		Long: `Show a quick summary of the EC2 instance, including its metadata and a
high-level network overview. Fields that cannot be retrieved (for example
when the metadata service is unreachable) are reported as unavailable.`,
		Example: "  vpc-proof status",
		GroupID: "diagnostics",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd)
			if app == nil {
				return fmt.Errorf("status: application is not initialized")
			}
			if app.logger != nil {
				app.logger.Info("status command invoked", observability.Component("cli"))
			}

			ctx := cmd.Context()
			info := app.fetchInstanceInfo(ctx)
			route := app.fetchRouteSummary(ctx)

			cmd.Printf("%-18s : %s\n", "Instance ID", metadataValue(info.InstanceID, info.MetadataError != nil))
			cmd.Printf("%-18s : %s\n", "Availability Zone", metadataValue(info.AvailabilityZone, info.MetadataError != nil))
			cmd.Printf("%-18s : %s\n", "Private IP", metadataValue(info.PrivateIP, info.MetadataError != nil))
			cmd.Printf("%-18s : %s\n", "Public IP", metadataValue(info.PublicIP, info.MetadataError != nil))

			switch {
			case !route.Available:
				cmd.Printf("%-18s : %s\n", "Default Route", "unavailable")
			case route.Gateway == "":
				cmd.Printf("%-18s : %s\n", "Default Route", "absent")
			default:
				cmd.Printf("%-18s : %s\n", "Default Route", fmt.Sprintf("present (gateway %s via %s)", route.Gateway, route.Interface))
			}
			return nil
		},
	}
}

// metadataValue renders a metadata field, degrading gracefully when the
// metadata service is unreachable.
func metadataValue(value string, unavailable bool) string {
	if unavailable {
		return "unavailable"
	}
	if value == "" {
		return "none"
	}
	return value
}

// newCheckCommand runs the full probe suite.
func newCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run the full network probe suite",
		Long: `Run every network and metadata probe and report the combined results.
This command acts as a CI/CD gateway: it exits with code 0 when the overall
status is pass or skip, code 1 when any probe fails, and code 2 when only
warnings are raised.`,
		Example: "  vpc-proof check",
		GroupID: "diagnostics",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd)
			if app == nil {
				return fmt.Errorf("check: application is not initialized")
			}

			report := app.RunProbes(cmd.Context())
			printProbeSummary(cmd.OutOrStdout(), report)

			switch report.Status {
			case probe.StatusFail:
				return exitError(exitCodeFailure, "check: %d probe(s) failed", countStatus(report, probe.StatusFail))
			case probe.StatusWarn:
				return exitError(exitCodeWarn, "check: %d probe(s) raised warnings", countStatus(report, probe.StatusWarn))
			default:
				return nil
			}
		},
	}
}

// newDiagnoseCommand runs probes and outputs troubleshooting hints.
func newDiagnoseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose",
		Short: "Run probes and output troubleshooting hints",
		Long: `Run the probe suite and translate any failure into actionable AWS
troubleshooting hints.`,
		Example: "  vpc-proof diagnose",
		GroupID: "diagnostics",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd)
			if app == nil {
				return fmt.Errorf("diagnose: application is not initialized")
			}

			report := app.RunProbes(cmd.Context())
			printProbeSummary(cmd.OutOrStdout(), report)

			hints := app.Diagnose(report)
			if len(hints) == 0 {
				cmd.Println("No issues detected.")
				return nil
			}

			cmd.Println("Troubleshooting hints:")
			for _, hint := range hints {
				cmd.Printf("  - [%s] %s\n", hint.Severity, hint.Message)
			}
			return nil
		},
	}
}

// printProbeSummary prints one line per probe result followed by a counts
// summary.
func printProbeSummary(w io.Writer, report probe.Report) {
	for _, result := range report.Results {
		fmt.Fprintf(w, "  %-22s %s\n", result.ID, result.Status)
	}
	fmt.Fprintf(w, "Summary: %d probes: %d pass, %d fail, %d warn, %d skip (overall %s)\n",
		len(report.Results),
		countStatus(report, probe.StatusPass),
		countStatus(report, probe.StatusFail),
		countStatus(report, probe.StatusWarn),
		countStatus(report, probe.StatusSkip),
		report.Status,
	)
}

// countStatus counts results with the given status.
func countStatus(report probe.Report, status probe.Status) int {
	count := 0
	for _, result := range report.Results {
		if result.Status == status {
			count++
		}
	}
	return count
}
