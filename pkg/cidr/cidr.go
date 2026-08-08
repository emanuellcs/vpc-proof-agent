package cidr

import (
	"fmt"
	"net/netip"
)

// ParseCIDR parses a CIDR string and returns its canonical (masked) prefix.
func ParseCIDR(cidr string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	return prefix.Masked(), nil
}

// IsValid reports whether cidr is a valid, canonical CIDR string.
func IsValid(cidr string) bool {
	_, err := ParseCIDR(cidr)
	return err == nil
}

// Contains reports whether the IP address ip belongs to the CIDR block cidr.
//
// IPv4-mapped IPv6 addresses are unmapped before comparison so that a
// dotted-quad address always matches an IPv4 prefix.
func Contains(cidr, ip string) (bool, error) {
	prefix, err := ParseCIDR(cidr)
	if err != nil {
		return false, err
	}

	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false, fmt.Errorf("invalid IP %q: %w", ip, err)
	}

	return prefix.Contains(addr.Unmap()), nil
}
