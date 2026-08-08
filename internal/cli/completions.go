package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newCompletionsCommand generates shell auto-completion scripts.
func newCompletionsCommand() *cobra.Command {
	var shell string

	cmd := &cobra.Command{
		Use:   "completions",
		Short: "Generate a shell auto-completion script",
		Long: `Generate a shell auto-completion script for bash, zsh, fish, or
PowerShell and print it to stdout. Source the output in the target shell.`,
		Example: `  vpc-proof completions --shell bash
  vpc-proof completions --shell zsh > ~/.zsh/completions/_vpc-proof`,
		GroupID: "info",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := cmd.Root()
			out := cmd.OutOrStdout()
			switch shell {
			case "bash":
				return root.GenBashCompletion(out)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletion(out)
			default:
				return fmt.Errorf("unsupported shell %q (expected bash, zsh, fish, or powershell)", shell)
			}
		},
	}

	cmd.Flags().StringVar(&shell, "shell", "bash", "target shell: bash, zsh, fish, or powershell")

	return cmd
}
