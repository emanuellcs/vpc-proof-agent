package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// envPrefix prefixes every configuration environment variable.
const envPrefix = "VPC_PROOF_"

// Overrides carries optional values that take precedence over every other
// source. They are provided by command-line flags; a nil field means the
// corresponding value was not supplied.
type Overrides struct {
	// LogLevel overrides LogConfig.Level.
	LogLevel *string
	// LogFormat overrides LogConfig.Format.
	LogFormat *string
	// ServerAddr overrides ServerConfig.Addr.
	ServerAddr *string
	// ServerPort overrides ServerConfig.Port.
	ServerPort *int
}

// LoadOptions configures the behavior of Load.
type LoadOptions struct {
	// ConfigFile is the path of a YAML configuration file. When empty, no
	// file is loaded and the remaining sources are defaults, environment
	// variables, and overrides.
	ConfigFile string
	// Overrides, when non-nil, are applied after the file and environment.
	Overrides *Overrides
}

// Load builds a Config by applying sources in precedence order: defaults,
// the YAML file (when a path is provided), environment variables, and
// command-line overrides.
//
// Hard failures (unreadable file, malformed YAML, unknown keys, or malformed
// environment values) are returned as a wrapped error. Semantic problems are
// not detected here; call Validate afterwards.
func Load(opts LoadOptions) (*Config, error) {
	cfg := Defaults()

	if opts.ConfigFile != "" {
		if err := applyFile(cfg, opts.ConfigFile); err != nil {
			return nil, err
		}
	}

	if err := applyEnv(cfg); err != nil {
		return nil, err
	}

	if opts.Overrides != nil {
		applyOverrides(cfg, opts.Overrides)
	}

	return cfg, nil
}

// applyFile merges the YAML document at path into cfg. Strict decoding
// rejects unknown keys so typos fail loudly.
func applyFile(cfg *Config, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	return nil
}

// envBinding maps a VPC_PROOF_ environment variable to a typed setter.
type envBinding struct {
	name  string
	apply func(value string) error
}

// applyEnv overlays every set VPC_PROOF_ environment variable onto cfg.
func applyEnv(cfg *Config) error {
	bindings := []envBinding{
		{name: envPrefix + "SERVER_ADDR", apply: setStr(&cfg.Server.Addr)},
		{name: envPrefix + "SERVER_PORT", apply: setInt(&cfg.Server.Port)},
		{name: envPrefix + "SERVER_READ_TIMEOUT", apply: setDuration(&cfg.Server.ReadTimeout)},
		{name: envPrefix + "SERVER_WRITE_TIMEOUT", apply: setDuration(&cfg.Server.WriteTimeout)},
		{name: envPrefix + "SERVER_IDLE_TIMEOUT", apply: setDuration(&cfg.Server.IdleTimeout)},
		{name: envPrefix + "AUTH_ENABLED", apply: setBool(&cfg.Auth.Enabled)},
		{name: envPrefix + "AUTH_TOKEN", apply: setStr(&cfg.Auth.Token)},
		{name: envPrefix + "PROBES_VPC_CIDR", apply: setStr(&cfg.Probes.VpcCIDR)},
		{name: envPrefix + "PROBES_SUBNET_CIDR", apply: setStr(&cfg.Probes.SubnetCIDR)},
		{name: envPrefix + "PROBES_ECHO_URLS", apply: setStrSlice(&cfg.Probes.EchoURLs)},
		{name: envPrefix + "PROBES_TIMEOUT", apply: setDuration(&cfg.Probes.Timeout)},
		{name: envPrefix + "PROBES_MAX_RETRIES", apply: setInt(&cfg.Probes.MaxRetries)},
		{name: envPrefix + "CACHE_PROBE_TTL", apply: setDuration(&cfg.Cache.ProbeTTL)},
		{name: envPrefix + "RATELIMIT_REQUESTS_PER_MINUTE", apply: setInt(&cfg.RateLimit.RequestsPerMinute)},
		{name: envPrefix + "RATELIMIT_BURST", apply: setInt(&cfg.RateLimit.Burst)},
		{name: envPrefix + "LOG_LEVEL", apply: setStr(&cfg.Log.Level)},
		{name: envPrefix + "LOG_FORMAT", apply: setStr(&cfg.Log.Format)},
	}

	for _, b := range bindings {
		value, ok := os.LookupEnv(b.name)
		if !ok {
			continue
		}
		if err := b.apply(value); err != nil {
			return fmt.Errorf("environment variable %s: %w", b.name, err)
		}
	}
	return nil
}

// applyOverrides applies command-line override values onto cfg.
func applyOverrides(cfg *Config, o *Overrides) {
	if o.LogLevel != nil {
		cfg.Log.Level = *o.LogLevel
	}
	if o.LogFormat != nil {
		cfg.Log.Format = *o.LogFormat
	}
	if o.ServerAddr != nil {
		cfg.Server.Addr = *o.ServerAddr
	}
	if o.ServerPort != nil {
		cfg.Server.Port = *o.ServerPort
	}
}

func setStr(dst *string) func(string) error {
	return func(value string) error {
		*dst = value
		return nil
	}
}

func setInt(dst *int) func(string) error {
	return func(value string) error {
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer %q", value)
		}
		*dst = v
		return nil
	}
}

func setBool(dst *bool) func(string) error {
	return func(value string) error {
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean %q", value)
		}
		*dst = v
		return nil
	}
}

func setDuration(dst *Duration) func(string) error {
	return func(value string) error {
		v, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration %q", value)
		}
		*dst = Duration(v)
		return nil
	}
}

func setStrSlice(dst *[]string) func(string) error {
	return func(value string) error {
		parts := strings.Split(value, ",")
		cleaned := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				cleaned = append(cleaned, p)
			}
		}
		*dst = cleaned
		return nil
	}
}
