// Package cli defines the command-line interface for the vpc-proof agent,
// built with the Cobra framework.
//
// The root command owns the global persistent flags (--config, --log-level,
// --log-format) and bootstraps every subcommand: it loads and validates the
// configuration, initializes the structured logger, builds the application
// container (metadata client, probe runner, diagnostic engine), and injects
// it into the command context so subcommands can consume it without
// re-parsing.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

// Process exit codes.
const (
	// exitCodeOK indicates success.
	exitCodeOK = 0
	// exitCodeFailure is the generic failure code and the `check` failure code.
	exitCodeFailure = 1
	// exitCodeWarn is the `check` warnings-only code.
	exitCodeWarn = 2
)

// envConfig is the environment variable that can point at the config file.
const envConfig = "VPC_PROOF_CONFIG"

// Execute runs the vpc-proof CLI with the standard streams and returns the
// process exit code.
func Execute() int {
	return execute(os.Args[1:], os.Stdout, os.Stderr, appDeps{})
}

// execute runs the CLI with explicit arguments, streams, and dependencies.
// It is the testable entry point; Execute wraps it with the process's
// arguments, streams, and production dependencies.
func execute(args []string, stdout, stderr io.Writer, deps appDeps) int {
	cmd := newRootCommand(deps)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitCodeFor(err)
	}
	return exitCodeOK
}

// exitCodeError carries an explicit process exit code.
type exitCodeError struct {
	code int
	msg  string
}

// Error returns the human-readable message.
func (e *exitCodeError) Error() string {
	return e.msg
}

// ExitCode returns the process exit code.
func (e *exitCodeError) ExitCode() int {
	return e.code
}

// exitError builds an exitCodeError.
func exitError(code int, format string, args ...any) error {
	return &exitCodeError{code: code, msg: fmt.Sprintf(format, args...)}
}

// exitCodeFor maps an error to a process exit code.
func exitCodeFor(err error) int {
	var exitErr *exitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.code
	}
	return exitCodeFailure
}

// newRootCommand builds the root command and its full command tree.
func newRootCommand(deps appDeps) *cobra.Command {
	var cfgFile string
	var logLevel string
	var logFormat string

	root := &cobra.Command{
		Use:   "vpc-proof",
		Short: "VPC Proof Agent - AWS networking diagnostic and evidence-gathering tool",
		Long: `vpc-proof is a diagnostic and evidence-gathering tool that validates and
proves that a manually provisioned AWS networking environment is functioning
correctly. It runs on an EC2 instance and offers a local CLI for
administration and a public REST API for reachability evidence.

The tool does not provision AWS resources: it observes, probes, and reports.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return bootstrap(cmd, deps)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "path to a YAML configuration file (overrides VPC_PROOF_CONFIG)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level: debug, info, warn, error (overrides config)")
	root.PersistentFlags().StringVar(&logFormat, "log-format", "", "log format: json or console (overrides config)")

	root.AddGroup(
		&cobra.Group{ID: "diagnostics", Title: "Diagnostics"},
		&cobra.Group{ID: "administration", Title: "Administration"},
		&cobra.Group{ID: "info", Title: "Info"},
	)

	root.AddCommand(
		newVersionCommand(),
		newValidateConfigCommand(),
		newStatusCommand(),
		newCheckCommand(),
		newDiagnoseCommand(),
		newReportCommand(),
		newServeCommand(),
	)

	return root
}

// bootstrap loads and validates the configuration, initializes the logger,
// builds the application container, and injects it into the command context.
// Any failure aborts the command chain and is reported to the user.
func bootstrap(cmd *cobra.Command, deps appDeps) error {
	cfg, errs, err := loadConfiguration(cmd)
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	if len(errs) > 0 {
		return validationErrors(errs)
	}

	logger, err := observability.New(cfg.Log.Level, cfg.Log.Format, cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	app, err := buildApp(cfg, logger, deps)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}

	withAppContext(cmd, app)
	return nil
}

// loadConfiguration resolves the config file path (flag, then environment,
// then default candidates), loads the configuration, and validates it. It
// never aborts on validation failures; the caller decides how to react.
func loadConfiguration(cmd *cobra.Command) (*config.Config, []error, error) {
	overrides := overridesFromFlags(cmd)

	path, _ := cmd.Flags().GetString("config")
	if path == "" {
		path = os.Getenv(envConfig)
	}
	if path == "" {
		path = findDefaultConfigFile()
	}

	cfg, err := config.Load(config.LoadOptions{ConfigFile: path, Overrides: overrides})
	if err != nil {
		return nil, nil, err
	}
	errs := cfg.Validate()
	return cfg, errs, nil
}

// overridesFromFlags collects only the flags that were explicitly set by the
// user, so that absent flags never shadow file or environment values.
func overridesFromFlags(cmd *cobra.Command) *config.Overrides {
	overrides := &config.Overrides{}
	flags := cmd.Flags()

	if flags.Changed("log-level") {
		if v, err := flags.GetString("log-level"); err == nil {
			overrides.LogLevel = &v
		}
	}
	if flags.Changed("log-format") {
		if v, err := flags.GetString("log-format"); err == nil {
			overrides.LogFormat = &v
		}
	}

	return overrides
}

// findDefaultConfigFile returns the first existing default config file, or
// an empty string when none exists.
func findDefaultConfigFile() string {
	candidates := []string{"vpc-proof.yaml"}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "vpc-proof", "config.yaml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "vpc-proof", "config.yaml"))
	}
	candidates = append(candidates, "/etc/vpc-proof/config.yaml")

	for _, p := range candidates {
		// #nosec G703 -- candidate config paths are user-controlled by design.
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// validationErrors renders the consolidated list of validation errors as a
// single, readable error.
func validationErrors(errs []error) error {
	var sb strings.Builder
	sb.WriteString("configuration is invalid:")
	for _, e := range errs {
		sb.WriteString("\n  - ")
		sb.WriteString(e.Error())
	}
	return errors.New(sb.String())
}
