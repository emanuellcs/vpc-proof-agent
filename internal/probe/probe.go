// Package probe implements the core network and metadata validation logic
// of the vpc-proof agent.
//
// A Probe performs a single diagnostic check. The Runner executes a set of
// probes, enforcing per-probe and global context timeouts, and aggregates
// the individual results into a Report. Every external interaction (IMDS,
// DNS, HTTP, the routing table, and interface discovery) is behind an
// interface, so probes are fully testable with fakes and local servers.
package probe

import "context"

// Probe identifiers, used by diagnostics rules and reports.
const (
	MetadataProbeID            = "metadata"
	VPCOwnershipProbeID        = "vpc_ownership"
	SubnetOwnershipProbeID     = "subnet_ownership"
	DefaultRouteProbeID        = "default_route"
	DNSProbeID                 = "dns"
	InternetHTTPSProbeID       = "internet_https"
	PublicIPConsistencyProbeID = "public_ip_consistency"
)

// Probe executes a single diagnostic check.
type Probe interface {
	// ID returns the stable probe identifier.
	ID() string
	// Execute runs the probe, honoring context cancellation and timeouts,
	// and returns its result.
	Execute(ctx context.Context) Result
}
