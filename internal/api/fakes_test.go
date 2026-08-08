package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/api/cache"
	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/diagnostic"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
	"github.com/emanuellcs/vpc-proof-agent/pkg/netutil"
)

// doRequest performs an HTTP request with optional headers.
func doRequest(t *testing.T, method, url string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// newRequest builds a minimal request with the given RemoteAddr.
func newRequest(remoteAddr string) *http.Request {
	return &http.Request{
		Method:     http.MethodGet,
		RemoteAddr: remoteAddr,
		Header:     http.Header{},
	}
}

// readBody drains and returns the response body.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

// freeAPIPort reserves then releases a TCP port so a server can bind to it.
func freeAPIPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// waitAPIPort polls until the port accepts connections.
func waitAPIPort(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not start listening")
}

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
}

func (f fakeResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return f.addrs, nil
}

// fakeRouteReader implements netutil.RouteTableReader.
type fakeRouteReader struct {
	routes []netutil.Route
}

func (f fakeRouteReader) ReadRouteTable(context.Context) ([]netutil.Route, error) {
	return f.routes, nil
}

// fakeInterfaceProvider implements netutil.InterfaceProvider.
type fakeInterfaceProvider struct {
	interfaces []netutil.Interface
}

func (f fakeInterfaceProvider) Interfaces(context.Context) ([]netutil.Interface, error) {
	return f.interfaces, nil
}

// testMeta builds the fake metadata with a healthy instance profile.
func testMeta() fakeMetadata {
	return fakeMetadata{
		instanceID: "i-0123456789abcdef0",
		privateIP:  "10.0.1.42",
		publicIP:   "203.0.113.7",
		az:         "us-east-1a",
	}
}

// newTestRunner builds a probe runner whose every external interaction is
// served by fakes and the given echo server.
func newTestRunner(echoURL string, echoClient *http.Client) *probe.Runner {
	meta := testMeta()

	probes := []probe.Probe{
		probe.NewMetadataProbe(meta, nil),
		probe.NewVPCOwnershipProbe(meta, "10.0.0.0/16", nil),
		probe.NewSubnetOwnershipProbe(meta, "10.0.1.0/24", nil),
		probe.NewDefaultRouteProbe(
			fakeRouteReader{routes: []netutil.Route{
				{Interface: "eth0", Destination: netip.IPv4Unspecified(), Gateway: netip.MustParseAddr("10.0.2.2"), Flags: 0x3},
			}},
			fakeInterfaceProvider{interfaces: []netutil.Interface{
				{Name: "eth0", Flags: net.FlagUp, Addrs: []netip.Addr{netip.MustParseAddr("10.0.1.42")}},
			}},
			nil,
		),
		probe.NewDNSProbe(fakeResolver{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.10")}}, "amazon.com", nil),
		probe.NewInternetHTTPSProbe(echoClient, echoURL, 0, 0, nil),
		probe.NewPublicIPConsistencyProbe(meta, echoClient, echoURL, 0, nil),
	}

	return probe.NewRunner(probes)
}

// newTestServer builds a fully wired API server backed by fakes. The mutate
// callback lets tests tweak the configuration before the server is built.
func newTestServer(t *testing.T, mutate func(*config.Config)) (*Server, *cache.Cache, *httptest.Server) {
	t.Helper()

	cfg := config.Defaults()
	if mutate != nil {
		mutate(cfg)
	}

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("203.0.113.7"))
	}))
	t.Cleanup(echoServer.Close)

	probeCache := cache.New(cfg.Cache.ProbeTTL.Value())
	server, err := New(Options{
		Config:   cfg,
		Logger:   nil,
		Metadata: testMeta(),
		Runner:   newTestRunner(echoServer.URL, echoServer.Client()),
		Engine:   diagnostic.New(),
		Cache:    probeCache,
	})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	return server, probeCache, httpServer
}
