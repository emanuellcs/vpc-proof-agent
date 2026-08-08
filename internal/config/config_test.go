package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	c := Defaults()

	if c.Server.Addr != "0.0.0.0" {
		t.Errorf("Server.Addr = %q, want %q", c.Server.Addr, "0.0.0.0")
	}
	if c.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", c.Server.Port)
	}
	if got := c.Server.ReadTimeout.Value(); got != 10*time.Second {
		t.Errorf("Server.ReadTimeout = %s, want 10s", got)
	}
	if got := c.Server.WriteTimeout.Value(); got != 10*time.Second {
		t.Errorf("Server.WriteTimeout = %s, want 10s", got)
	}
	if got := c.Server.IdleTimeout.Value(); got != 60*time.Second {
		t.Errorf("Server.IdleTimeout = %s, want 60s", got)
	}

	if c.Auth.Enabled {
		t.Error("Auth.Enabled = true, want false (fail-safe default)")
	}
	if len(c.Auth.PublicPaths) != 3 {
		t.Errorf("Auth.PublicPaths has %d entries, want 3", len(c.Auth.PublicPaths))
	}

	if c.Probes.VpcCIDR != "10.0.0.0/16" {
		t.Errorf("Probes.VpcCIDR = %q, want %q", c.Probes.VpcCIDR, "10.0.0.0/16")
	}
	if c.Probes.SubnetCIDR != "10.0.1.0/24" {
		t.Errorf("Probes.SubnetCIDR = %q, want %q", c.Probes.SubnetCIDR, "10.0.1.0/24")
	}
	if len(c.Probes.EchoURLs) != 1 || c.Probes.EchoURLs[0] != "https://checkip.amazonaws.com" {
		t.Errorf("Probes.EchoURLs = %v, want single checkip URL", c.Probes.EchoURLs)
	}
	if c.Probes.DNSHost != "amazon.com" {
		t.Errorf("Probes.DNSHost = %q, want %q", c.Probes.DNSHost, "amazon.com")
	}
	if got := c.Probes.Timeout.Value(); got != 5*time.Second {
		t.Errorf("Probes.Timeout = %s, want 5s", got)
	}
	if c.Probes.MaxRetries != 2 {
		t.Errorf("Probes.MaxRetries = %d, want 2", c.Probes.MaxRetries)
	}

	if got := c.Cache.ProbeTTL.Value(); got != 5*time.Minute {
		t.Errorf("Cache.ProbeTTL = %s, want 5m", got)
	}

	if c.RateLimit.RequestsPerMinute != 100 {
		t.Errorf("RateLimit.RequestsPerMinute = %d, want 100", c.RateLimit.RequestsPerMinute)
	}
	if c.RateLimit.Burst != 20 {
		t.Errorf("RateLimit.Burst = %d, want 20", c.RateLimit.Burst)
	}

	if c.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", c.Log.Level, "info")
	}
	if c.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", c.Log.Format, "json")
	}
}

func TestDefaultsAreValid(t *testing.T) {
	if errs := Defaults().Validate(); len(errs) != 0 {
		t.Fatalf("defaults should be valid, got errors: %v", errs)
	}
}

func TestDurationString(t *testing.T) {
	d := Duration(1500 * time.Millisecond)
	if got, want := d.String(), "1.5s"; got != want {
		t.Errorf("Duration.String() = %q, want %q", got, want)
	}
}

func TestDurationNonPositive(t *testing.T) {
	zero := Duration(0)
	if !zero.NonPositive() {
		t.Error("Duration(0).NonPositive() = false, want true")
	}
	negative := Duration(-time.Second)
	if !negative.NonPositive() {
		t.Error("Duration(-1s).NonPositive() = false, want true")
	}
	positive := Duration(time.Second)
	if positive.NonPositive() {
		t.Error("Duration(1s).NonPositive() = true, want false")
	}
}

func TestDurationRoundTrip(t *testing.T) {
	want := Duration(2*time.Minute + 30*time.Second)
	got, err := parseDurationString(want.String())
	if err != nil {
		t.Fatalf("parseDurationString(%q): %v", want.String(), err)
	}
	if got != want {
		t.Errorf("round trip = %s, want %s", got.String(), want.String())
	}
}

func parseDurationString(s string) (Duration, error) {
	v, err := time.ParseDuration(s)
	return Duration(v), err
}
