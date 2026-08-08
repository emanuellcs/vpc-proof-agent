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

	stdout, _, err := runCLI("validate-config", "--config", path)
	if err != nil {
		t.Fatalf("validate-config returned error: %v", err)
	}
	if !strings.Contains(stdout, "configuration is valid") {
		t.Errorf("expected success message, got %q", stdout)
	}
}

func TestValidateConfigInvalid(t *testing.T) {
	path := writeTempConfig(t, "server:\n  port: 70000\n")

	_, stderr, err := runCLI("validate-config", "--config", path)
	if err == nil {
		t.Fatal("expected error for invalid configuration, got nil")
	}
	if !strings.Contains(stderr, "server.port") {
		t.Errorf("stderr should mention server.port, got %q", stderr)
	}
	if !strings.Contains(stderr, "65535") {
		t.Errorf("stderr should explain the port range, got %q", stderr)
	}
}

func TestValidateConfigMissingFile(t *testing.T) {
	_, stderr, err := runCLI("validate-config", "--config", "/no/such/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
	if !strings.Contains(stderr, "configuration") {
		t.Errorf("stderr should mention configuration, got %q", stderr)
	}
}

func TestReportFlags(t *testing.T) {
	stdout, _, err := runCLI("report", "--format", "json", "--output", "-")
	if err != nil {
		t.Fatalf("report returned error: %v", err)
	}
	if !strings.Contains(stdout, "[stub] report") {
		t.Errorf("stdout should contain stub notice, got %q", stdout)
	}
	if !strings.Contains(stdout, "format=json") {
		t.Errorf("stdout should carry the format, got %q", stdout)
	}
	if !strings.Contains(stdout, "output=stdout") {
		t.Errorf("stdout should resolve - to stdout, got %q", stdout)
	}
}

func TestReportInvalidFormat(t *testing.T) {
	_, stderr, err := runCLI("report", "--format", "xml")
	if err == nil {
		t.Fatal("expected error for invalid report format, got nil")
	}
	if !strings.Contains(stderr, "invalid report format") {
		t.Errorf("stderr should mention the invalid format, got %q", stderr)
	}
}

func TestServeFlags(t *testing.T) {
	stdout, _, err := runCLI("serve", "--addr", "127.0.0.1", "--port", "9090")
	if err != nil {
		t.Fatalf("serve returned error: %v", err)
	}
	if !strings.Contains(stdout, "127.0.0.1:9090") {
		t.Errorf("stdout should show the overridden listen address, got %q", stdout)
	}
}

func TestServeUsesConfigDefaults(t *testing.T) {
	stdout, _, err := runCLI("serve")
	if err != nil {
		t.Fatalf("serve returned error: %v", err)
	}
	if !strings.Contains(stdout, "0.0.0.0:8080") {
		t.Errorf("stdout should show the default listen address, got %q", stdout)
	}
}

func TestStubCommands(t *testing.T) {
	for _, cmd := range []string{"status", "check", "diagnose"} {
		t.Run(cmd, func(t *testing.T) {
			stdout, _, err := runCLI(cmd)
			if err != nil {
				t.Fatalf("%s returned error: %v", cmd, err)
			}
			if !strings.Contains(stdout, "[stub] "+cmd) {
				t.Errorf("%s stdout should contain stub notice, got %q", cmd, stdout)
			}
		})
	}
}
