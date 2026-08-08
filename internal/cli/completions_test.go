package cli

import (
	"strings"
	"testing"
)

func TestCompletionsCommand(t *testing.T) {
	tests := []struct {
		shell string
		want  string
	}{
		{shell: "bash", want: "__start_vpc-proof"},
		{shell: "zsh", want: "#compdef"},
		{shell: "fish", want: "complete -c vpc-proof"},
		{shell: "powershell", want: "Register-ArgumentCompleter"},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			stdout, _, code := runCLI("completions", "--shell", tt.shell)
			if code != exitCodeOK {
				t.Fatalf("completions --shell %s exit code = %d, want 0", tt.shell, code)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Errorf("completions --shell %s missing %q", tt.shell, tt.want)
			}
		})
	}
}

func TestCompletionsCommandInvalidShell(t *testing.T) {
	_, stderr, code := runCLI("completions", "--shell", "tcsh")
	if code == exitCodeOK {
		t.Fatal("expected a non-zero exit code for an unsupported shell")
	}
	if !strings.Contains(stderr, "unsupported shell") {
		t.Errorf("stderr should mention the unsupported shell, got %q", stderr)
	}
}

func TestCompletionsCommandDefaultShell(t *testing.T) {
	stdout, _, code := runCLI("completions")
	if code != exitCodeOK {
		t.Fatalf("completions exit code = %d, want 0 (default bash)", code)
	}
	if !strings.Contains(stdout, "__start_vpc-proof") {
		t.Error("default shell should be bash")
	}
}
