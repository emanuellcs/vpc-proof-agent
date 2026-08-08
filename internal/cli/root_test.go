package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// runCLI executes the CLI with production dependencies.
func runCLI(args ...string) (stdout, stderr string, code int) {
	return runCLIWith(&appDeps{}, args...)
}

// runCLIWith executes the CLI with injected dependencies.
func runCLIWith(deps *appDeps, args ...string) (stdout, stderr string, code int) {
	var out, errBuf bytes.Buffer
	code = execute(args, &out, &errBuf, deps)
	return out.String(), errBuf.String(), code
}

func TestRootHelp(t *testing.T) {
	stdout, _, code := runCLI("--help")
	if code != exitCodeOK {
		t.Fatalf("--help exit code = %d, want 0", code)
	}
	for _, want := range []string{"Usage:", "validate-config", "serve", "report", "version", "status", "check", "diagnose"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help output missing %q", want)
		}
	}
}

func TestRootBareInvocationShowsUsage(t *testing.T) {
	stdout, _, code := runCLI()
	if code != exitCodeOK {
		t.Fatalf("bare invocation exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("bare invocation should print usage, got %q", stdout)
	}
}

func TestVersionCommand(t *testing.T) {
	stdout, _, code := runCLI("version")
	if code != exitCodeOK {
		t.Fatalf("version exit code = %d, want 0", code)
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
	_, stderr, code := runCLI("--config", "/nonexistent/vpc-proof/config.yaml", "status")
	if code != exitCodeFailure {
		t.Fatalf("exit code = %d, want %d", code, exitCodeFailure)
	}
	if !strings.Contains(stderr, "configuration") {
		t.Errorf("stderr should mention configuration, got %q", stderr)
	}
}

func TestInvalidLogLevelOverride(t *testing.T) {
	_, stderr, code := runCLI("--log-level", "verbose", "status")
	if code != exitCodeFailure {
		t.Fatalf("exit code = %d, want %d", code, exitCodeFailure)
	}
	if !strings.Contains(stderr, "log.level") {
		t.Errorf("stderr should mention log.level, got %q", stderr)
	}
}
