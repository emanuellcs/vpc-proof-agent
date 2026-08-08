// Package config centralizes configuration loading and validation for the
// vpc-proof agent.
//
// Configuration is resolved with the following precedence (highest first):
//
//  1. command-line flags (Overrides)
//  2. environment variables prefixed with VPC_PROOF_
//  3. a YAML configuration file
//  4. built-in defaults
//
// The loader is deliberately dependency-light and explicit: every source is
// applied in order and every parsed value is type-checked. Validation is
// performed separately by (*Config).Validate and reports a consolidated list
// of errors so the CLI can present them all at once.
package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that is YAML-aware.
//
// Durations are expressed in configuration files and environment variables
// as strings parseable by time.ParseDuration (for example "5s", "500ms", or
// "2m30s"), which keeps the format human-friendly and unambiguous.
type Duration time.Duration

// UnmarshalYAML decodes a YAML scalar into a Duration.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("must be a duration string such as \"5s\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalYAML encodes a Duration as its string form.
func (d *Duration) MarshalYAML() (any, error) {
	return d.String(), nil
}

// String returns the canonical string representation of the duration.
func (d *Duration) String() string {
	return time.Duration(*d).String()
}

// Value returns the duration as a time.Duration.
func (d *Duration) Value() time.Duration {
	return time.Duration(*d)
}

// NonPositive reports whether the duration is zero or negative.
func (d *Duration) NonPositive() bool {
	return time.Duration(*d) <= 0
}

// Config is the root configuration for the vpc-proof agent.
type Config struct {
	// Server configures the public REST API HTTP server.
	Server ServerConfig `yaml:"server" json:"server"`
	// Auth configures API authentication and endpoint exposure.
	Auth AuthConfig `yaml:"auth" json:"auth"`
	// Probes configures the network and metadata validation probes.
	Probes ProbesConfig `yaml:"probes" json:"probes"`
	// Cache configures caching of expensive probe results.
	Cache CacheConfig `yaml:"cache" json:"cache"`
	// RateLimit configures per-client API rate limiting.
	RateLimit RateLimitConfig `yaml:"ratelimit" json:"ratelimit"`
	// Log configures structured logging output.
	Log LogConfig `yaml:"log" json:"log"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Addr is the interface to bind, for example "0.0.0.0".
	Addr string `yaml:"addr" json:"addr"`
	// Port is the TCP port to listen on.
	Port int `yaml:"port" json:"port"`
	// ReadTimeout bounds reading the request body.
	ReadTimeout Duration `yaml:"read_timeout" json:"read_timeout"`
	// WriteTimeout bounds writing the response.
	WriteTimeout Duration `yaml:"write_timeout" json:"write_timeout"`
	// IdleTimeout bounds keeping a connection idle.
	IdleTimeout Duration `yaml:"idle_timeout" json:"idle_timeout"`
	// ShutdownTimeout bounds the graceful shutdown window for in-flight
	// requests.
	ShutdownTimeout Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
}

// AuthConfig holds API authentication settings.
type AuthConfig struct {
	// Enabled turns token-based authentication on or off.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Token is the shared secret required by authenticated endpoints.
	// Required when Enabled is true.
	Token string `yaml:"token" json:"token"`
	// PublicPaths lists endpoint paths that bypass authentication, for
	// example "/healthz", "/readyz", and "/metrics".
	PublicPaths []string `yaml:"public_paths" json:"public_paths"`
}

// ProbesConfig holds settings for the network and metadata probes.
type ProbesConfig struct {
	// VpcCIDR is the expected VPC CIDR, for example "10.0.0.0/16".
	VpcCIDR string `yaml:"vpc_cidr" json:"vpc_cidr"`
	// SubnetCIDR is the expected subnet CIDR, for example "10.0.1.0/24".
	SubnetCIDR string `yaml:"subnet_cidr" json:"subnet_cidr"`
	// EchoURLs are external services used to discover the public IP.
	EchoURLs []string `yaml:"echo_urls" json:"echo_urls"`
	// DNSHost is the hostname resolved to verify DNS works, for example
	// "amazon.com".
	DNSHost string `yaml:"dns_host" json:"dns_host"`
	// Timeout bounds each individual probe.
	Timeout Duration `yaml:"timeout" json:"timeout"`
	// MaxRetries is the number of retries for transient probe failures.
	MaxRetries int `yaml:"max_retries" json:"max_retries"`
}

// CacheConfig holds caching settings.
type CacheConfig struct {
	// ProbeTTL is how long expensive probe results are cached.
	ProbeTTL Duration `yaml:"probe_ttl" json:"probe_ttl"`
}

// RateLimitConfig holds per-client rate limiting settings.
type RateLimitConfig struct {
	// RequestsPerMinute is the steady-state request budget per client.
	RequestsPerMinute int `yaml:"requests_per_minute" json:"requests_per_minute"`
	// Burst is the maximum burst of requests allowed in a short window.
	Burst int `yaml:"burst" json:"burst"`
}

// LogConfig holds structured logging settings.
type LogConfig struct {
	// Level is one of debug, info, warn, or error.
	Level string `yaml:"level" json:"level"`
	// Format is either "json" or "console".
	Format string `yaml:"format" json:"format"`
}

// Defaults returns a Config populated with the project's canonical defaults.
//
// The returned configuration always passes Validate, so the absence of any
// external configuration source is not an error.
func Defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:            "0.0.0.0",
			Port:            8080,
			ReadTimeout:     Duration(10 * time.Second),
			WriteTimeout:    Duration(10 * time.Second),
			IdleTimeout:     Duration(60 * time.Second),
			ShutdownTimeout: Duration(10 * time.Second),
		},
		Auth: AuthConfig{
			Enabled:     false,
			PublicPaths: []string{"/healthz", "/readyz", "/metrics"},
		},
		Probes: ProbesConfig{
			VpcCIDR:    "10.0.0.0/16",
			SubnetCIDR: "10.0.1.0/24",
			EchoURLs:   []string{"https://checkip.amazonaws.com"},
			DNSHost:    "amazon.com",
			Timeout:    Duration(5 * time.Second),
			MaxRetries: 2,
		},
		Cache: CacheConfig{
			ProbeTTL: Duration(5 * time.Minute),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: 100,
			Burst:             20,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
