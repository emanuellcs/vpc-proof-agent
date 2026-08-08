package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/pkg/netutil"
)

// fakeMetadata implements metadata.Client with canned values and errors.
type fakeMetadata struct {
	values map[string]string
	errs   map[string]error
}

func (f fakeMetadata) get(key string) (string, error) {
	if err := f.errs[key]; err != nil {
		return "", err
	}
	return f.values[key], nil
}

func (f fakeMetadata) InstanceID(ctx context.Context) (string, error) { return f.get("instance-id") }
func (f fakeMetadata) PrivateIP(ctx context.Context) (string, error)  { return f.get("private-ip") }
func (f fakeMetadata) PublicIP(ctx context.Context) (string, error)   { return f.get("public-ip") }
func (f fakeMetadata) AvailabilityZone(ctx context.Context) (string, error) {
	return f.get("az")
}
func (f fakeMetadata) MACAddress(ctx context.Context) (string, error) { return f.get("mac") }
func (f fakeMetadata) SubnetCIDR(ctx context.Context) (string, error) { return f.get("subnet-cidr") }
func (f fakeMetadata) VpcCIDR(ctx context.Context) (string, error)    { return f.get("vpc-cidr") }

func metadataValues() map[string]string {
	return map[string]string{
		"instance-id": "i-0123456789abcdef0",
		"private-ip":  "10.0.1.42",
		"public-ip":   "203.0.113.7",
		"az":          "us-east-1a",
		"mac":         "0a:1b:2c:3d:4e:5f",
		"subnet-cidr": "10.0.1.0/24",
		"vpc-cidr":    "10.0.0.0/16",
	}
}

// fakeResolver implements netutil.Resolver.
type fakeResolver struct {
	addrs []netip.Addr
	err   error
}

func (f fakeResolver) LookupNetIP(_ context.Context, _ string, _ string) ([]netip.Addr, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.addrs, nil
}

// fakeRouteReader implements netutil.RouteTableReader.
type fakeRouteReader struct {
	routes []netutil.Route
	err    error
}

func (f fakeRouteReader) ReadRouteTable(context.Context) ([]netutil.Route, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.routes, nil
}

// fakeInterfaceProvider implements netutil.InterfaceProvider.
type fakeInterfaceProvider struct {
	interfaces []netutil.Interface
	err        error
}

func (f fakeInterfaceProvider) Interfaces(context.Context) ([]netutil.Interface, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.interfaces, nil
}

func defaultRouteSet() []netutil.Route {
	return []netutil.Route{
		{Interface: "eth0", Destination: netip.IPv4Unspecified(), Gateway: netip.MustParseAddr("10.0.2.2"), Flags: 0x3},
		{Interface: "eth0", Destination: netip.MustParseAddr("10.0.2.0"), Gateway: netip.Addr{}, Flags: 0x1},
	}
}

func eth0Interfaces() []netutil.Interface {
	return []netutil.Interface{
		{Name: "eth0", Flags: net.FlagUp, Addrs: []netip.Addr{netip.MustParseAddr("10.0.1.42")}},
	}
}

// compile-time interface checks.
var (
	_ Probe = (*MetadataProbe)(nil)
	_ Probe = (*VPCOwnershipProbe)(nil)
	_ Probe = (*SubnetOwnershipProbe)(nil)
	_ Probe = (*DefaultRouteProbe)(nil)
	_ Probe = (*DNSProbe)(nil)
	_ Probe = (*InternetHTTPSProbe)(nil)
	_ Probe = (*PublicIPConsistencyProbe)(nil)
	_ Probe = (*SystemResourcesProbe)(nil)
	_ Probe = (*ClockSkewProbe)(nil)
)

func TestMetadataProbe(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		result := NewMetadataProbe(client, nil).Execute(context.Background())

		if result.Status != StatusPass {
			t.Fatalf("status = %s, want pass", result.Status)
		}
		if result.Details["instance_id"] != "i-0123456789abcdef0" {
			t.Errorf("instance_id detail missing: %v", result.Details)
		}
		if result.Details["public_ip"] != "203.0.113.7" {
			t.Errorf("public_ip detail missing: %v", result.Details)
		}
		if result.ID != MetadataProbeID {
			t.Errorf("ID = %q, want %q", result.ID, MetadataProbeID)
		}
	})

	t.Run("fail on metadata error", func(t *testing.T) {
		client := fakeMetadata{errs: map[string]error{"instance-id": context.DeadlineExceeded}}
		result := NewMetadataProbe(client, nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
		if result.Hint != metadataHint {
			t.Errorf("hint = %q, want metadata hint", result.Hint)
		}
	})
}

func TestVPCOwnershipProbe(t *testing.T) {
	t.Run("pass inside vpc", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		result := NewVPCOwnershipProbe(client, "10.0.0.0/16", nil).Execute(context.Background())

		if result.Status != StatusPass {
			t.Fatalf("status = %s, want pass: %s", result.Status, result.Message)
		}
	})

	t.Run("fail outside vpc", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		result := NewVPCOwnershipProbe(client, "192.168.0.0/16", nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
		if result.Hint != vpcOwnershipHint {
			t.Errorf("hint = %q, want vpc ownership hint", result.Hint)
		}
		if result.Details["private_ip"] != "10.0.1.42" {
			t.Errorf("private_ip detail missing: %v", result.Details)
		}
	})

	t.Run("fail on metadata error", func(t *testing.T) {
		client := fakeMetadata{errs: map[string]error{"private-ip": context.Canceled}}
		result := NewVPCOwnershipProbe(client, "10.0.0.0/16", nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
	})

	t.Run("fail on invalid cidr", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		result := NewVPCOwnershipProbe(client, "not-a-cidr", nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
	})
}

func TestSubnetOwnershipProbe(t *testing.T) {
	t.Run("pass inside subnet", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		result := NewSubnetOwnershipProbe(client, "10.0.1.0/24", nil).Execute(context.Background())

		if result.Status != StatusPass {
			t.Fatalf("status = %s, want pass", result.Status)
		}
	})

	t.Run("fail outside subnet", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		result := NewSubnetOwnershipProbe(client, "10.0.2.0/24", nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
		if result.Hint != subnetOwnershipHint {
			t.Errorf("hint = %q, want subnet ownership hint", result.Hint)
		}
	})

	t.Run("fail on metadata error", func(t *testing.T) {
		client := fakeMetadata{errs: map[string]error{"private-ip": context.Canceled}}
		result := NewSubnetOwnershipProbe(client, "10.0.1.0/24", nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
	})
}

func TestDefaultRouteProbe(t *testing.T) {
	t.Run("pass with default route", func(t *testing.T) {
		reader := fakeRouteReader{routes: defaultRouteSet()}
		provider := fakeInterfaceProvider{interfaces: eth0Interfaces()}
		result := NewDefaultRouteProbe(reader, provider, nil).Execute(context.Background())

		if result.Status != StatusPass {
			t.Fatalf("status = %s, want pass: %s", result.Status, result.Message)
		}
		if result.Details["gateway"] != "10.0.2.2" {
			t.Errorf("gateway detail = %q, want 10.0.2.2", result.Details["gateway"])
		}
		if result.Details["interface"] != "eth0" {
			t.Errorf("interface detail = %q, want eth0", result.Details["interface"])
		}
		if result.Details["primary_ip"] != "10.0.1.42" {
			t.Errorf("primary_ip detail = %q, want 10.0.1.42", result.Details["primary_ip"])
		}
	})

	t.Run("pass without interface provider", func(t *testing.T) {
		reader := fakeRouteReader{routes: defaultRouteSet()}
		result := NewDefaultRouteProbe(reader, nil, nil).Execute(context.Background())

		if result.Status != StatusPass {
			t.Fatalf("status = %s, want pass", result.Status)
		}
	})

	t.Run("fail without default route", func(t *testing.T) {
		reader := fakeRouteReader{routes: []netutil.Route{
			{Interface: "eth0", Destination: netip.MustParseAddr("10.0.2.0"), Gateway: netip.Addr{}, Flags: 0x1},
		}}
		result := NewDefaultRouteProbe(reader, nil, nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
		if result.Hint != defaultRouteHint {
			t.Errorf("hint = %q, want default route hint", result.Hint)
		}
	})

	t.Run("fail on read error", func(t *testing.T) {
		reader := fakeRouteReader{err: context.Canceled}
		result := NewDefaultRouteProbe(reader, nil, nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
	})
}

func TestDNSProbe(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		resolver := fakeResolver{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.10")}}
		result := NewDNSProbe(resolver, "amazon.com", nil).Execute(context.Background())

		if result.Status != StatusPass {
			t.Fatalf("status = %s, want pass", result.Status)
		}
		if result.Details["host"] != "amazon.com" {
			t.Errorf("host detail missing: %v", result.Details)
		}
		if result.Details["addresses"] != "203.0.113.10" {
			t.Errorf("addresses detail = %q", result.Details["addresses"])
		}
	})

	t.Run("fail on resolution error", func(t *testing.T) {
		resolver := fakeResolver{err: context.DeadlineExceeded}
		result := NewDNSProbe(resolver, "amazon.com", nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
		if result.Hint != dnsHint {
			t.Errorf("hint = %q, want dns hint", result.Hint)
		}
	})
}

func newEchoServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestInternetHTTPSProbe(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		server := newEchoServer(t, "203.0.113.7", http.StatusOK)
		result := NewInternetHTTPSProbe(server.Client(), server.URL, 0, 0, nil).Execute(context.Background())

		if result.Status != StatusPass {
			t.Fatalf("status = %s, want pass: %s", result.Status, result.Message)
		}
		if result.Details["external_ip"] != "203.0.113.7" {
			t.Errorf("external_ip detail = %q", result.Details["external_ip"])
		}
	})

	t.Run("fail on non-2xx", func(t *testing.T) {
		server := newEchoServer(t, "unavailable", http.StatusServiceUnavailable)
		result := NewInternetHTTPSProbe(server.Client(), server.URL, 0, 0, nil).Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
		if result.Hint != internetHTTPSHint {
			t.Errorf("hint = %q, want internet hint", result.Hint)
		}
	})

	t.Run("fail on connection error", func(t *testing.T) {
		result := NewInternetHTTPSProbe(&http.Client{Timeout: time.Second}, "http://127.0.0.1:1", 0, 0, nil).
			Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
	})
}

func TestPublicIPConsistencyProbe(t *testing.T) {
	t.Run("pass when IPs match", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		server := newEchoServer(t, "203.0.113.7", http.StatusOK)
		result := NewPublicIPConsistencyProbe(client, server.Client(), server.URL, 0, nil).
			Execute(context.Background())

		if result.Status != StatusPass {
			t.Fatalf("status = %s, want pass: %s", result.Status, result.Message)
		}
	})

	t.Run("fail when IPs differ", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		server := newEchoServer(t, "198.51.100.9", http.StatusOK)
		result := NewPublicIPConsistencyProbe(client, server.Client(), server.URL, 0, nil).
			Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
		if result.Hint != publicIPHint {
			t.Errorf("hint = %q, want public ip hint", result.Hint)
		}
	})

	t.Run("warn when metadata has no public ip", func(t *testing.T) {
		values := metadataValues()
		values["public-ip"] = ""
		client := fakeMetadata{values: values}
		server := newEchoServer(t, "203.0.113.7", http.StatusOK)
		result := NewPublicIPConsistencyProbe(client, server.Client(), server.URL, 0, nil).
			Execute(context.Background())

		if result.Status != StatusWarn {
			t.Fatalf("status = %s, want warn", result.Status)
		}
		if result.Hint != publicIPHint {
			t.Errorf("hint = %q, want public ip hint", result.Hint)
		}
	})

	t.Run("warn when echo returns empty ip", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		server := newEchoServer(t, "", http.StatusOK)
		result := NewPublicIPConsistencyProbe(client, server.Client(), server.URL, 0, nil).
			Execute(context.Background())

		if result.Status != StatusWarn {
			t.Fatalf("status = %s, want warn", result.Status)
		}
	})

	t.Run("fail on metadata error", func(t *testing.T) {
		client := fakeMetadata{errs: map[string]error{"public-ip": context.Canceled}}
		server := newEchoServer(t, "203.0.113.7", http.StatusOK)
		result := NewPublicIPConsistencyProbe(client, server.Client(), server.URL, 0, nil).
			Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
	})

	t.Run("fail on echo error", func(t *testing.T) {
		client := fakeMetadata{values: metadataValues()}
		result := NewPublicIPConsistencyProbe(client, &http.Client{Timeout: time.Second}, "http://127.0.0.1:1", 0, nil).
			Execute(context.Background())

		if result.Status != StatusFail {
			t.Fatalf("status = %s, want fail", result.Status)
		}
		if !strings.Contains(result.Message, "echo") {
			t.Errorf("message should mention the echo service, got %q", result.Message)
		}
	})
}
