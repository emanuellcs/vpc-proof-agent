package netutil

import (
	"context"
	"fmt"
	"net"
	"net/netip"
)

// Interface is a platform-neutral description of a network interface.
type Interface struct {
	// Name is the OS interface name (for example "eth0").
	Name string
	// Flags carries the net.Interface flags of the interface.
	Flags net.Flags
	// Addrs holds the interface's assigned addresses, unmapped.
	Addrs []netip.Addr
}

// InterfaceProvider abstracts enumeration of network interfaces so the
// discovery logic can be tested with injected data instead of the host's
// actual network state.
type InterfaceProvider interface {
	// Interfaces returns all network interfaces.
	Interfaces(ctx context.Context) ([]Interface, error)
}

// OSInterfaceProvider enumerates interfaces from the operating system.
type OSInterfaceProvider struct{}

// Interfaces lists the host's network interfaces via package net.
func (OSInterfaceProvider) Interfaces(ctx context.Context) ([]Interface, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	raw, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}

	interfaces := make([]Interface, 0, len(raw))
	for _, item := range raw {
		addresses, err := item.Addrs()
		if err != nil {
			continue
		}
		iface := Interface{Name: item.Name, Flags: item.Flags}
		for _, addr := range addresses {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if parsed, ok := netip.AddrFromSlice(ip); ok {
				iface.Addrs = append(iface.Addrs, parsed.Unmap())
			}
		}
		interfaces = append(interfaces, iface)
	}
	return interfaces, nil
}

// FirstIPv4 returns the first non-loopback IPv4 address of an interface.
func FirstIPv4(iface Interface) (netip.Addr, bool) {
	for _, addr := range iface.Addrs {
		if addr.Is4() && !addr.IsLoopback() {
			return addr, true
		}
	}
	return netip.Addr{}, false
}

// PrimaryInterface resolves the primary network interface.
//
// It prefers the interface named by the default route (preferredName), then
// the first up, non-loopback interface carrying an IPv4 address, and finally
// the first up, non-loopback interface.
func PrimaryInterface(ctx context.Context, provider InterfaceProvider, preferredName string) (Interface, error) {
	interfaces, err := provider.Interfaces(ctx)
	if err != nil {
		return Interface{}, err
	}

	if preferredName != "" {
		for _, iface := range interfaces {
			if iface.Name == preferredName {
				return iface, nil
			}
		}
	}

	for _, iface := range interfaces {
		if !isUsable(iface) {
			continue
		}
		if _, ok := FirstIPv4(iface); ok {
			return iface, nil
		}
	}

	for _, iface := range interfaces {
		if !isUsable(iface) {
			continue
		}
		return iface, nil
	}

	return Interface{}, fmt.Errorf("no primary interface found")
}

// isUsable reports whether an interface is up and not a loopback.
func isUsable(iface Interface) bool {
	return iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0
}
