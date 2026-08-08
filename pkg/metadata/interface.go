package metadata

import "context"

// Client fetches EC2 instance metadata over IMDSv2.
//
// The interface is consumed by the probe engine and can be satisfied by the
// concrete client in this package (pointed at the real IMDS endpoint or a
// local mock) or by a hand-written fake in tests.
type Client interface {
	// InstanceID returns the EC2 instance ID.
	InstanceID(ctx context.Context) (string, error)
	// PrivateIP returns the primary private IPv4 address.
	PrivateIP(ctx context.Context) (string, error)
	// PublicIP returns the public IPv4 address, or an empty string when the
	// instance has no public IP.
	PublicIP(ctx context.Context) (string, error)
	// AvailabilityZone returns the placement availability zone.
	AvailabilityZone(ctx context.Context) (string, error)
	// MACAddress returns the primary network interface MAC address.
	MACAddress(ctx context.Context) (string, error)
	// SubnetCIDR returns the CIDR block of the primary subnet.
	SubnetCIDR(ctx context.Context) (string, error)
	// VpcCIDR returns the CIDR block of the VPC.
	VpcCIDR(ctx context.Context) (string, error)
}
