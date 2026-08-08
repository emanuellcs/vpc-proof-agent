package cli

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"testing"

	"github.com/emanuellcs/vpc-proof-agent/pkg/netutil"
)

// fakeMetadata implements metadata.Client with canned values.
type fakeMetadata struct {
	instanceID, privateIP, publicIP, az string
	err                                 error
}

func (f fakeMetadata) InstanceID(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.instanceID, nil
}

func (f fakeMetadata) PrivateIP(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.privateIP, nil
}

func (f fakeMetadata) PublicIP(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.publicIP, nil
}

func (f fakeMetadata) AvailabilityZone(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.az, nil
}

func (f fakeMetadata) MACAddress(context.Context) (string, error) { return "", nil }
func (f fakeMetadata) SubnetCIDR(context.Context) (string, error) { return "", nil }
func (f fakeMetadata) VpcCIDR(context.Context) (string, error)    { return "", nil }

// fakeResolver implements netutil.Resolver.
type fakeResolver struct {
	addrs []netip.Addr
	err   error
}

func (f fakeResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
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

// defaultDeps builds a dependency set that makes every probe pass, together
// with an echo server returning 203.0.113.7.
func defaultDeps(t *testing.T) (appDeps, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("203.0.113.7"))
	}))
	t.Cleanup(server.Close)

	deps := appDeps{
		metadataClient: fakeMetadata{
			instanceID: "i-0123456789abcdef0",
			privateIP:  "10.0.1.42",
			publicIP:   "203.0.113.7",
			az:         "us-east-1a",
		},
		resolver: fakeResolver{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.10")}},
		routeReader: fakeRouteReader{routes: []netutil.Route{
			{Interface: "eth0", Destination: netip.IPv4Unspecified(), Gateway: netip.MustParseAddr("10.0.2.2"), Flags: 0x3},
		}},
		interfaceProvider: fakeInterfaceProvider{interfaces: []netutil.Interface{
			{Name: "eth0", Flags: net.FlagUp, Addrs: []netip.Addr{netip.MustParseAddr("10.0.1.42")}},
		}},
		echoHTTPClient: server.Client(),
		fileReader: func(path string) ([]byte, error) {
			switch path {
			case "/proc/uptime":
				return []byte("12345.67 43210.98\n"), nil
			case "/proc/loadavg":
				return []byte("0.50 0.30 0.20 1/234 567\n"), nil
			case "/proc/meminfo":
				return []byte("MemTotal:       1000000 kB\nMemFree:         950000 kB\nMemAvailable:    950000 kB\n"), nil
			default:
				return nil, os.ErrNotExist
			}
		},
	}
	return deps, server
}
