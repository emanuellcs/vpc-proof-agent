package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestLoadUsesDefaultsWithoutSources(t *testing.T) {
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load with no sources: %v", err)
	}
	if cfg.Server.Port != 8080 || cfg.Log.Level != "info" {
		t.Fatalf("expected defaults, got port=%d level=%q", cfg.Server.Port, cfg.Log.Level)
	}
	if errs := cfg.Validate(); len(errs) != 0 {
		t.Fatalf("no-source config should validate, got %v", errs)
	}
}

func TestLoadFromFile(t *testing.T) {
	path := writeTempConfig(t, `
server:
  addr: 127.0.0.1
  port: 9090
  read_timeout: 3s
log:
  level: debug
`)

	cfg, err := Load(LoadOptions{ConfigFile: path})
	if err != nil {
		t.Fatalf("Load from file: %v", err)
	}
	if cfg.Server.Addr != "127.0.0.1" {
		t.Errorf("Server.Addr = %q, want 127.0.0.1", cfg.Server.Addr)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if got := cfg.Server.ReadTimeout.Value(); got != 3*time.Second {
		t.Errorf("Server.ReadTimeout = %s, want 3s", got)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want debug", cfg.Log.Level)
	}
	// Fields absent from the file must retain defaults.
	if cfg.Server.IdleTimeout.Value() != 60*time.Second {
		t.Errorf("Server.IdleTimeout = %s, want default 60s", cfg.Server.IdleTimeout.String())
	}
	if cfg.Probes.VpcCIDR != "10.0.0.0/16" {
		t.Errorf("Probes.VpcCIDR = %q, want default 10.0.0.0/16", cfg.Probes.VpcCIDR)
	}
}

func TestLoadFromFileRejectsUnknownKeys(t *testing.T) {
	path := writeTempConfig(t, `
server:
  port: 8080
  bogus_field: true
`)
	_, err := Load(LoadOptions{ConfigFile: path})
	if err == nil {
		t.Fatal("expected error for unknown config key, got nil")
	}
}

func TestLoadFromFileMissingExplicitPath(t *testing.T) {
	_, err := Load(LoadOptions{ConfigFile: "/nonexistent/vpc-proof/config.yaml"})
	if err == nil {
		t.Fatal("expected error for missing explicit config file, got nil")
	}
}

func TestLoadFromFileInvalidDuration(t *testing.T) {
	path := writeTempConfig(t, `
cache:
  probe_ttl: banana
`)
	_, err := Load(LoadOptions{ConfigFile: path})
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
	if !strings.Contains(err.Error(), `invalid duration "banana"`) {
		t.Errorf("error should surface the bad duration, got %v", err)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("VPC_PROOF_SERVER_PORT", "9000")
	t.Setenv("VPC_PROOF_LOG_LEVEL", "warn")
	t.Setenv("VPC_PROOF_PROBES_VPC_CIDR", "172.16.0.0/12")
	t.Setenv("VPC_PROOF_PROBES_DNS_HOST", "aws.amazon.com")
	t.Setenv("VPC_PROOF_PROBES_ECHO_URLS", "https://api.ipify.org, https://checkip.amazonaws.com")
	t.Setenv("VPC_PROOF_SERVER_READ_TIMEOUT", "2s")
	t.Setenv("VPC_PROOF_SERVER_SHUTDOWN_TIMEOUT", "20s")
	t.Setenv("VPC_PROOF_HISTORY_MAX_ENTRIES", "10")
	t.Setenv("VPC_PROOF_HISTORY_DISK_PATH", "/tmp/history.json")
	t.Setenv("VPC_PROOF_HISTORY_FLUSH_INTERVAL", "15s")

	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load with env: %v", err)
	}
	if cfg.Server.Port != 9000 {
		t.Errorf("Server.Port = %d, want 9000", cfg.Server.Port)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want warn", cfg.Log.Level)
	}
	if cfg.Probes.VpcCIDR != "172.16.0.0/12" {
		t.Errorf("Probes.VpcCIDR = %q, want 172.16.0.0/12", cfg.Probes.VpcCIDR)
	}
	if cfg.Probes.DNSHost != "aws.amazon.com" {
		t.Errorf("Probes.DNSHost = %q, want aws.amazon.com", cfg.Probes.DNSHost)
	}
	if len(cfg.Probes.EchoURLs) != 2 {
		t.Fatalf("Probes.EchoURLs = %v, want 2 entries", cfg.Probes.EchoURLs)
	}
	if got := cfg.Server.ReadTimeout.Value(); got != 2*time.Second {
		t.Errorf("Server.ReadTimeout = %s, want 2s", got)
	}
	if got := cfg.Server.ShutdownTimeout.Value(); got != 20*time.Second {
		t.Errorf("Server.ShutdownTimeout = %s, want 20s", got)
	}
	if cfg.History.MaxEntries != 10 {
		t.Errorf("History.MaxEntries = %d, want 10", cfg.History.MaxEntries)
	}
	if cfg.History.DiskPath != "/tmp/history.json" {
		t.Errorf("History.DiskPath = %q", cfg.History.DiskPath)
	}
	if got := cfg.History.FlushInterval.Value(); got != 15*time.Second {
		t.Errorf("History.FlushInterval = %s, want 15s", got)
	}
}

func TestLoadEnvInvalidInt(t *testing.T) {
	t.Setenv("VPC_PROOF_SERVER_PORT", "not-a-number")
	_, err := Load(LoadOptions{})
	if err == nil {
		t.Fatal("expected error for invalid integer env value, got nil")
	}
	if !strings.Contains(err.Error(), "SERVER_PORT") {
		t.Errorf("error should mention SERVER_PORT, got %v", err)
	}
}

func TestLoadEnvInvalidBool(t *testing.T) {
	t.Setenv("VPC_PROOF_AUTH_ENABLED", "yes-please")
	_, err := Load(LoadOptions{})
	if err == nil {
		t.Fatal("expected error for invalid boolean env value, got nil")
	}
}

func TestLoadEnvInvalidDuration(t *testing.T) {
	t.Setenv("VPC_PROOF_CACHE_PROBE_TTL", "soon")
	_, err := Load(LoadOptions{})
	if err == nil {
		t.Fatal("expected error for invalid duration env value, got nil")
	}
}

func TestPrecedenceFileEnvOverride(t *testing.T) {
	path := writeTempConfig(t, "server:\n  port: 8000\n")

	t.Run("file only", func(t *testing.T) {
		cfg, err := Load(LoadOptions{ConfigFile: path})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Server.Port != 8000 {
			t.Errorf("port = %d, want 8000 (file)", cfg.Server.Port)
		}
	})

	t.Run("env overrides file", func(t *testing.T) {
		t.Setenv("VPC_PROOF_SERVER_PORT", "9000")
		cfg, err := Load(LoadOptions{ConfigFile: path})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Server.Port != 9000 {
			t.Errorf("port = %d, want 9000 (env over file)", cfg.Server.Port)
		}
	})

	t.Run("override over env over file", func(t *testing.T) {
		t.Setenv("VPC_PROOF_SERVER_PORT", "9000")
		cfg, err := Load(LoadOptions{ConfigFile: path, Overrides: &Overrides{ServerPort: intPtr(9500)}})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Server.Port != 9500 {
			t.Errorf("port = %d, want 9500 (flag over env over file)", cfg.Server.Port)
		}
	})

	t.Run("override only", func(t *testing.T) {
		cfg, err := Load(LoadOptions{Overrides: &Overrides{LogFormat: strPtr("console")}})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Log.Format != "console" {
			t.Errorf("Log.Format = %q, want console", cfg.Log.Format)
		}
		if cfg.Server.Port != 8080 {
			t.Errorf("port = %d, want default 8080", cfg.Server.Port)
		}
	})
}

func TestLoadAuthTokenFromEnv(t *testing.T) {
	t.Setenv("VPC_PROOF_AUTH_ENABLED", "true")
	t.Setenv("VPC_PROOF_AUTH_TOKEN", "s3cret-token")

	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Auth.Enabled {
		t.Error("Auth.Enabled = false, want true")
	}
	if cfg.Auth.Token != "s3cret-token" {
		t.Errorf("Auth.Token = %q, want s3cret-token", cfg.Auth.Token)
	}
}
