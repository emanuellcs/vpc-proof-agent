package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// runCLI executes the CLI with the given arguments, capturing output and the
// returned error.
func runCLI(args ...string) (stdout, stderr string, err error) {
	var out, errBuf bytes.Buffer
	err = execute(args, &out, &errBuf)
	return out.String(), errBuf.String(), err
}

func TestRootHelp(t *testing.T) {
	stdout, _, err := runCLI("--help")
	if err != nil {
		t.Fatalf("--help returned error: %v", err)
	}
	for _, want := range []string{"Usage:", "validate-config", "serve", "report", "version"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help output missing %q", want)
		}
	}
}

func TestRootBareInvocationShowsUsage(t *testing.T) {
	stdout, _, err := runCLI()
	if err != nil {
		t.Fatalf("bare invocation returned error: %v", err)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("bare invocation should print usage, got %q", stdout)
	}
}

func TestVersionCommand(t *testing.T) {
	stdout, _, err := runCLI("version")
	if err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	for _, want := range []string{
		"version:",
		"go version: " + runtime.Version(),
		runtime.GOOS + "/" + runtime.GOARCH,
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("version output missing %q in %q", want, stdout)
		}
	}
}

func TestPersistentPreRunConfigError(t *testing.T) {
	_, stderr, err := runCLI("--config", "/nonexistent/vpc-proof/config.yaml", "status")
	if err == nil {
		t.Fatal("expected error for missing explicit config file, got nil")
	}
	if !strings.Contains(stderr, "configuration") {
		t.Errorf("stderr should mention configuration, got %q", stderr)
	}
}

func TestInvalidLogLevelOverride(t *testing.T) {
	_, stderr, err := runCLI("--log-level", "verbose", "status")
	if err == nil {
		t.Fatal("expected error for invalid log level override, got nil")
	}
	if !strings.Contains(stderr, "log.level") {
		t.Errorf("stderr should mention log.level, got %q", stderr)
	}
}

func TestBootstrapInjectsContext(t *testing.T) {
	stdout, stderr, err := runCLI("status")
	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	if !strings.Contains(stdout, "[stub] status") {
		t.Errorf("stdout should contain stub notice, got %q", stdout)
	}
	if !strings.Contains(stderr, `"level":"info"`) {
		t.Errorf("logger from context should have emitted to stderr, got %q", stderr)
	}
}
