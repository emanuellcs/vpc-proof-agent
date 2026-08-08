package probe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/pkg/netutil"
)

// dnsHint is attached when DNS resolution fails.
const dnsHint = "Check the VPC DNS settings and DHCP option sets."

// DNSProbe verifies that a known public hostname can be resolved with the
// system resolver.
type DNSProbe struct {
	resolver netutil.Resolver
	host     string
	logger   *observability.Logger
}

// NewDNSProbe builds a DNSProbe for the given host.
func NewDNSProbe(resolver netutil.Resolver, host string, logger *observability.Logger) *DNSProbe {
	return &DNSProbe{resolver: resolver, host: host, logger: logger}
}

// ID returns the probe identifier.
func (p *DNSProbe) ID() string {
	return DNSProbeID
}

// Execute resolves the configured hostname.
func (p *DNSProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	result := Result{
		ID:      p.ID(),
		Name:    "DNS resolution",
		Details: map[string]string{},
	}

	addrs, err := netutil.Resolve(ctx, p.resolver, p.host)
	if err != nil {
		result.Status = StatusFail
		result.Duration = time.Since(start)
		result.Message = fmt.Sprintf("could not resolve %s: %v", p.host, err)
		result.Hint = dnsHint
		result.Details["host"] = p.host
		return result
	}

	result.Status = StatusPass
	result.Duration = time.Since(start)
	result.Message = fmt.Sprintf("resolved %s", p.host)
	result.Details["host"] = p.host

	addresses := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addresses = append(addresses, addr.String())
	}
	result.Details["addresses"] = strings.Join(addresses, ", ")

	if p.logger != nil {
		p.logger.Debug("dns resolution verified",
			observability.Component("probe"),
			observability.Str("host", p.host),
			observability.Str("addresses", strings.Join(addresses, ", ")),
		)
	}
	return result
}
