package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/pkg/netutil"
)

// defaultRouteHint is attached when no default route is found.
const defaultRouteHint = "Verify the Route Table has a 0.0.0.0/0 route to the Internet Gateway and is associated with the subnet."

// DefaultRouteProbe verifies that the instance has an active default route
// and records the gateway, interface, and primary IPv4 address.
type DefaultRouteProbe struct {
	reader   netutil.RouteTableReader
	provider netutil.InterfaceProvider
	logger   *observability.Logger
}

// NewDefaultRouteProbe builds a DefaultRouteProbe. provider is optional and
// used only to enrich the result with the primary IPv4 address.
func NewDefaultRouteProbe(reader netutil.RouteTableReader, provider netutil.InterfaceProvider, logger *observability.Logger) *DefaultRouteProbe {
	return &DefaultRouteProbe{reader: reader, provider: provider, logger: logger}
}

// ID returns the probe identifier.
func (p *DefaultRouteProbe) ID() string {
	return DefaultRouteProbeID
}

// Execute reads the routing table and reports on the default route.
func (p *DefaultRouteProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	result := Result{
		ID:      p.ID(),
		Name:    "Default route",
		Details: map[string]string{},
	}

	routes, err := p.reader.ReadRouteTable(ctx)
	if err != nil {
		result.Status = StatusFail
		result.Duration = time.Since(start)
		result.Message = fmt.Sprintf("could not read the routing table: %v", err)
		result.Hint = defaultRouteHint
		return result
	}

	route, ok := netutil.DefaultRoute(routes)
	if !ok {
		result.Status = StatusFail
		result.Duration = time.Since(start)
		result.Message = "no active default route found"
		result.Hint = defaultRouteHint
		return result
	}

	result.Details["interface"] = route.Interface
	result.Details["gateway"] = route.Gateway.String()

	if p.provider != nil {
		if primary, err := netutil.PrimaryInterface(ctx, p.provider, route.Interface); err == nil {
			if ip, ok := netutil.FirstIPv4(primary); ok {
				result.Details["primary_ip"] = ip.String()
			}
		}
	}

	result.Status = StatusPass
	result.Duration = time.Since(start)
	result.Message = fmt.Sprintf("default route via %s (gateway %s)", route.Interface, route.Gateway)
	if p.logger != nil {
		p.logger.Debug("default route verified",
			observability.Component("probe"),
			observability.Str("interface", route.Interface),
			observability.Str("gateway", route.Gateway.String()),
		)
	}
	return result
}
