package config

import (
	"fmt"
	"net/netip"
	"net/url"
)

// Validate checks the configuration and returns a consolidated list of
// errors, one per invalid field. Every error message is prefixed with the
// fully qualified field name (for example "server.port") so it can be
// reported verbatim to the user.
func (c *Config) Validate() []error {
	var errs []error
	errs = append(errs, validateServer(c.Server)...)
	errs = append(errs, validateAuth(c.Auth)...)
	errs = append(errs, validateProbes(&c.Probes)...)
	errs = append(errs, validateCache(c.Cache)...)
	errs = append(errs, validateRateLimit(c.RateLimit)...)
	errs = append(errs, validateLog(c.Log)...)
	return errs
}

func validateServer(s ServerConfig) []error {
	var errs []error
	if s.Addr == "" {
		errs = append(errs, fmt.Errorf("server.addr: must not be empty"))
	}
	if s.Port < 1 || s.Port > 65535 {
		errs = append(errs, fmt.Errorf("server.port: must be between 1 and 65535, got %d", s.Port))
	}
	if s.ReadTimeout.NonPositive() {
		errs = append(errs, fmt.Errorf("server.read_timeout: must be a positive duration, got %s", s.ReadTimeout.String()))
	}
	if s.WriteTimeout.NonPositive() {
		errs = append(errs, fmt.Errorf("server.write_timeout: must be a positive duration, got %s", s.WriteTimeout.String()))
	}
	if s.IdleTimeout.NonPositive() {
		errs = append(errs, fmt.Errorf("server.idle_timeout: must be a positive duration, got %s", s.IdleTimeout.String()))
	}
	return errs
}

func validateAuth(a AuthConfig) []error {
	var errs []error
	if a.Enabled && a.Token == "" {
		errs = append(errs, fmt.Errorf("auth.token: must not be empty when auth is enabled"))
	}
	return errs
}

func validateProbes(p *ProbesConfig) []error {
	var errs []error
	if _, err := netip.ParsePrefix(p.VpcCIDR); err != nil {
		errs = append(errs, fmt.Errorf("probes.vpc_cidr: invalid CIDR %q", p.VpcCIDR))
	}
	if _, err := netip.ParsePrefix(p.SubnetCIDR); err != nil {
		errs = append(errs, fmt.Errorf("probes.subnet_cidr: invalid CIDR %q", p.SubnetCIDR))
	}
	if len(p.EchoURLs) == 0 {
		errs = append(errs, fmt.Errorf("probes.echo_urls: must contain at least one URL"))
	}
	for _, u := range p.EchoURLs {
		if !isHTTPURL(u) {
			errs = append(errs, fmt.Errorf("probes.echo_urls: invalid HTTP(S) URL %q", u))
		}
	}
	if p.DNSHost == "" {
		errs = append(errs, fmt.Errorf("probes.dns_host: must not be empty"))
	}
	if p.Timeout.NonPositive() {
		errs = append(errs, fmt.Errorf("probes.timeout: must be a positive duration, got %s", p.Timeout.String()))
	}
	if p.MaxRetries < 0 {
		errs = append(errs, fmt.Errorf("probes.max_retries: must not be negative, got %d", p.MaxRetries))
	}
	return errs
}

func validateCache(c CacheConfig) []error {
	var errs []error
	if c.ProbeTTL.NonPositive() {
		errs = append(errs, fmt.Errorf("cache.probe_ttl: must be a positive duration, got %s", c.ProbeTTL.String()))
	}
	return errs
}

func validateRateLimit(r RateLimitConfig) []error {
	var errs []error
	if r.RequestsPerMinute < 1 {
		errs = append(errs, fmt.Errorf("ratelimit.requests_per_minute: must be at least 1, got %d", r.RequestsPerMinute))
	}
	if r.Burst < 1 {
		errs = append(errs, fmt.Errorf("ratelimit.burst: must be at least 1, got %d", r.Burst))
	}
	return errs
}

func validateLog(l LogConfig) []error {
	var errs []error
	switch l.Level {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("log.level: must be one of debug, info, warn, error, got %q", l.Level))
	}
	switch l.Format {
	case "json", "console":
	default:
		errs = append(errs, fmt.Errorf("log.format: must be one of json, console, got %q", l.Format))
	}
	return errs
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
