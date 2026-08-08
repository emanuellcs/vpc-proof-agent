package netutil

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

type fakeProvider struct {
	interfaces []Interface
	err        error
}

func (f fakeProvider) Interfaces(ctx context.Context) ([]Interface, error) {
	if f.err != nil {
		return nil, f.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.interfaces, nil
}

func iface(name string, flags net.Flags, addrs ...string) Interface {
	parsed := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		parsed = append(parsed, netip.MustParseAddr(a))
	}
	return Interface{Name: name, Flags: flags, Addrs: parsed}
}

func TestPrimaryInterfacePrefersPreferredName(t *testing.T) {
	provider := fakeProvider{interfaces: []Interface{
		iface("eth0", net.FlagUp, "10.0.1.42"),
		iface("eth1", net.FlagUp, "10.0.2.42"),
	}}

	got, err := PrimaryInterface(context.Background(), provider, "eth1")
	if err != nil {
		t.Fatalf("PrimaryInterface: %v", err)
	}
	if got.Name != "eth1" {
		t.Errorf("primary interface = %q, want eth1", got.Name)
	}
}

func TestPrimaryInterfaceFallsBackToFirstUsable(t *testing.T) {
	provider := fakeProvider{interfaces: []Interface{
		iface("lo", net.FlagUp|net.FlagLoopback, "127.0.0.1"),
		iface("eth0", 0, "10.0.1.42"),
		iface("eth1", net.FlagUp, "10.0.1.43"),
	}}

	got, err := PrimaryInterface(context.Background(), provider, "")
	if err != nil {
		t.Fatalf("PrimaryInterface: %v", err)
	}
	if got.Name != "eth1" {
		t.Errorf("primary interface = %q, want eth1", got.Name)
	}
}

func TestPrimaryInterfacePrefersIPv4Carrier(t *testing.T) {
	provider := fakeProvider{interfaces: []Interface{
		iface("eth0", net.FlagUp, "fe80::1"),
		iface("eth1", net.FlagUp, "10.0.1.44"),
	}}

	got, err := PrimaryInterface(context.Background(), provider, "")
	if err != nil {
		t.Fatalf("PrimaryInterface: %v", err)
	}
	if got.Name != "eth1" {
		t.Errorf("primary interface = %q, want eth1", got.Name)
	}
}

func TestPrimaryInterfaceLastResortWithoutIPv4(t *testing.T) {
	provider := fakeProvider{interfaces: []Interface{
		iface("lo", net.FlagUp|net.FlagLoopback, "127.0.0.1"),
		iface("eth0", net.FlagUp, "fe80::1"),
	}}

	got, err := PrimaryInterface(context.Background(), provider, "")
	if err != nil {
		t.Fatalf("PrimaryInterface: %v", err)
	}
	if got.Name != "eth0" {
		t.Errorf("primary interface = %q, want eth0", got.Name)
	}
}

func TestPrimaryInterfaceNoUsable(t *testing.T) {
	provider := fakeProvider{interfaces: []Interface{
		iface("lo", net.FlagUp|net.FlagLoopback, "127.0.0.1"),
		iface("eth0", 0, "10.0.1.42"),
	}}

	if _, err := PrimaryInterface(context.Background(), provider, ""); err == nil {
		t.Fatal("expected error with no usable interface, got nil")
	}
}

func TestPrimaryInterfaceProviderError(t *testing.T) {
	provider := fakeProvider{err: errors.New("boom")}
	if _, err := PrimaryInterface(context.Background(), provider, ""); err == nil {
		t.Fatal("expected provider error to propagate, got nil")
	}
}

func TestFirstIPv4(t *testing.T) {
	tests := []struct {
		name   string
		iface  Interface
		want   string
		wantOK bool
	}{
		{name: "ipv4 present", iface: iface("eth0", net.FlagUp, "fe80::1", "10.0.1.42"), want: "10.0.1.42", wantOK: true},
		{name: "only ipv6", iface: iface("eth0", net.FlagUp, "fe80::1"), wantOK: false},
		{name: "only loopback ipv4", iface: iface("lo", net.FlagUp, "127.0.0.1"), wantOK: false},
		{name: "no addresses", iface: iface("eth0", net.FlagUp), wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FirstIPv4(tt.iface)
			if ok != tt.wantOK {
				t.Fatalf("FirstIPv4 ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && got.String() != tt.want {
				t.Errorf("FirstIPv4 = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOSInterfaceProvider(t *testing.T) {
	provider := OSInterfaceProvider{}

	interfaces, err := provider.Interfaces(context.Background())
	if err != nil {
		t.Fatalf("OSInterfaceProvider.Interfaces: %v", err)
	}
	if len(interfaces) == 0 {
		t.Error("expected at least the loopback interface")
	}
}

func TestOSInterfaceProviderCanceled(t *testing.T) {
	provider := OSInterfaceProvider{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := provider.Interfaces(ctx); err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}
