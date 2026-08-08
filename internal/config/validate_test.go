package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "defaults valid", mutate: nil, want: ""},
		{name: "server addr empty", mutate: func(c *Config) { c.Server.Addr = "" }, want: "server.addr"},
		{name: "server port zero", mutate: func(c *Config) { c.Server.Port = 0 }, want: "server.port"},
		{name: "server port negative", mutate: func(c *Config) { c.Server.Port = -1 }, want: "server.port"},
		{name: "server port too high", mutate: func(c *Config) { c.Server.Port = 65536 }, want: "server.port"},
		{name: "read timeout zero", mutate: func(c *Config) { c.Server.ReadTimeout = 0 }, want: "server.read_timeout"},
		{name: "write timeout negative", mutate: func(c *Config) { c.Server.WriteTimeout = Duration(-time.Second) }, want: "server.write_timeout"},
		{name: "idle timeout zero", mutate: func(c *Config) { c.Server.IdleTimeout = 0 }, want: "server.idle_timeout"},
		{name: "shutdown timeout zero", mutate: func(c *Config) { c.Server.ShutdownTimeout = 0 }, want: "server.shutdown_timeout"},
		{name: "auth token empty when enabled", mutate: func(c *Config) { c.Auth.Enabled = true; c.Auth.Token = "" }, want: "auth.token"},
		{name: "auth disabled with empty token valid", mutate: func(c *Config) { c.Auth.Token = "" }, want: ""},
		{name: "vpc cidr invalid", mutate: func(c *Config) { c.Probes.VpcCIDR = "10.0.0.0" }, want: "probes.vpc_cidr"},
		{name: "subnet cidr invalid", mutate: func(c *Config) { c.Probes.SubnetCIDR = "not-a-cidr" }, want: "probes.subnet_cidr"},
		{name: "echo urls empty", mutate: func(c *Config) { c.Probes.EchoURLs = nil }, want: "probes.echo_urls"},
		{name: "echo url invalid", mutate: func(c *Config) { c.Probes.EchoURLs = []string{"not a url"} }, want: "probes.echo_urls"},
		{name: "dns host empty", mutate: func(c *Config) { c.Probes.DNSHost = "" }, want: "probes.dns_host"},
		{name: "probe timeout zero", mutate: func(c *Config) { c.Probes.Timeout = 0 }, want: "probes.timeout"},
		{name: "max retries negative", mutate: func(c *Config) { c.Probes.MaxRetries = -1 }, want: "probes.max_retries"},
		{name: "cache ttl zero", mutate: func(c *Config) { c.Cache.ProbeTTL = 0 }, want: "cache.probe_ttl"},
		{name: "ratelimit requests zero", mutate: func(c *Config) { c.RateLimit.RequestsPerMinute = 0 }, want: "ratelimit.requests_per_minute"},
		{name: "ratelimit burst zero", mutate: func(c *Config) { c.RateLimit.Burst = 0 }, want: "ratelimit.burst"},
		{name: "log level invalid", mutate: func(c *Config) { c.Log.Level = "verbose" }, want: "log.level"},
		{name: "log format invalid", mutate: func(c *Config) { c.Log.Format = "xml" }, want: "log.format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Defaults()
			if tt.mutate != nil {
				tt.mutate(c)
			}
			errs := c.Validate()
			if tt.want == "" {
				if len(errs) != 0 {
					t.Fatalf("expected no errors, got %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("expected an error containing %q, got none", tt.want)
			}
			for _, e := range errs {
				if strings.Contains(e.Error(), tt.want) {
					return
				}
			}
			t.Fatalf("no error contained %q, got %v", tt.want, errs)
		})
	}
}

func TestValidateCollectsMultipleErrors(t *testing.T) {
	c := Defaults()
	c.Server.Port = 0
	c.Log.Level = "verbose"
	c.Probes.VpcCIDR = "broken"
	c.Cache.ProbeTTL = 0

	errs := c.Validate()
	if len(errs) < 4 {
		t.Fatalf("expected at least 4 errors, got %d: %v", len(errs), errs)
	}

	for _, field := range []string{"server.port", "log.level", "probes.vpc_cidr", "cache.probe_ttl"} {
		found := false
		for _, e := range errs {
			if strings.Contains(e.Error(), field) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected an error mentioning %q", field)
		}
	}
}

func TestValidateReportsFieldAndReason(t *testing.T) {
	c := Defaults()
	c.Server.Port = 99999
	errs := c.Validate()

	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "server.port") && strings.Contains(e.Error(), "65535") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected server.port error to mention the allowed range, got %v", errs)
	}
}
