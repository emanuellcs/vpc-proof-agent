package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestValidateConfigValid(t *testing.T) {
	path := writeTempConfig(t, "server:\n  addr: 127.0.0.1\n  port: 9090\n")

	stdout, _, code := runCLI("validate-config", "--config", path)
	if code != exitCodeOK {
		t.Fatalf("validate-config exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "configuration is valid") {
		t.Errorf("expected success message, got %q", stdout)
	}
}

func TestValidateConfigInvalid(t *testing.T) {
	path := writeTempConfig(t, "server:\n  port: 70000\n")

	_, stderr, code := runCLI("validate-config", "--config", path)
	if code != exitCodeFailure {
		t.Fatalf("exit code = %d, want %d", code, exitCodeFailure)
	}
	if !strings.Contains(stderr, "server.port") {
		t.Errorf("stderr should mention server.port, got %q", stderr)
	}
	if !strings.Contains(stderr, "65535") {
		t.Errorf("stderr should explain the port range, got %q", stderr)
	}
}

func TestValidateConfigMissingFile(t *testing.T) {
	_, stderr, code := runCLI("validate-config", "--config", "/no/such/config.yaml")
	if code != exitCodeFailure {
		t.Fatalf("exit code = %d, want %d", code, exitCodeFailure)
	}
	if !strings.Contains(stderr, "configuration") {
		t.Errorf("stderr should mention configuration, got %q", stderr)
	}
}

func TestServeFlags(t *testing.T) {
	stdout, _, code := runCLI("serve", "--addr", "127.0.0.1", "--port", "9090")
	if code != exitCodeOK {
		t.Fatalf("serve exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "127.0.0.1:9090") {
		t.Errorf("stdout should show the overridden listen address, got %q", stdout)
	}
}

func TestServeUsesConfigDefaults(t *testing.T) {
	stdout, _, code := runCLI("serve")
	if code != exitCodeOK {
		t.Fatalf("serve exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "0.0.0.0:8080") {
		t.Errorf("stdout should show the default listen address, got %q", stdout)
	}
}

func TestReportInvalidFormat(t *testing.T) {
	_, stderr, code := runCLI("report", "--format", "xml")
	if code != exitCodeFailure {
		t.Fatalf("exit code = %d, want %d", code, exitCodeFailure)
	}
	if !strings.Contains(stderr, "unsupported report format") {
		t.Errorf("stderr should mention the invalid format, got %q", stderr)
	}
}
