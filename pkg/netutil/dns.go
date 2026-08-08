package netutil

import (
	"context"
	"fmt"
	"net"
	"net/netip"
)

// Resolver abstracts DNS hostname resolution so probes can be tested with a
// fake resolver instead of the system resolver.
type Resolver interface {
	// LookupNetIP resolves host to a list of addresses, as net.Resolver does.
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// compile-time assertion that the standard resolver satisfies the interface.
var _ Resolver = (*net.Resolver)(nil)

// Resolve resolves host to addresses using resolver.
func Resolve(ctx context.Context, resolver Resolver, host string) ([]netip.Addr, error) {
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	return addrs, nil
}
