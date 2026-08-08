package netutil

import (
	"context"
	"net/netip"
	"strings"
	"testing"
)

const sampleRouteTable = `Iface	Destination	Gateway	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0202000A	0003	0	0	0	00000000	0	0	0
eth0	0002000A	00000000	0001	0	0	0	FFFFFF00	0	0	0
lo	0000007F	00000000	0001	0	0	0	000000FF	0	0	0
`

func TestParseRouteTable(t *testing.T) {
	routes, err := ParseRouteTable(strings.NewReader(sampleRouteTable))
	if err != nil {
		t.Fatalf("ParseRouteTable: %v", err)
	}
	if len(routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(routes))
	}

	defaultRoute, ok := DefaultRoute(routes)
	if !ok {
		t.Fatal("expected a default route")
	}
	if defaultRoute.Interface != "eth0" {
		t.Errorf("default route interface = %q, want eth0", defaultRoute.Interface)
	}
	if defaultRoute.Gateway != netip.MustParseAddr("10.0.2.2") {
		t.Errorf("default route gateway = %s, want 10.0.2.2", defaultRoute.Gateway)
	}
	if !defaultRoute.IsUp() {
		t.Error("default route should be up")
	}
	if defaultRoute.IsDefault() != true {
		t.Error("default route should be flagged default")
	}

	if routes[1].Destination != netip.MustParseAddr("10.0.2.0") {
		t.Errorf("second route destination = %s, want 10.0.2.0", routes[1].Destination)
	}
	if !routes[1].IsUp() || routes[1].IsDefault() {
		t.Errorf("second route flags wrong: %+v", routes[1])
	}
}

func TestHexLEToAddr(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "00000000", want: "0.0.0.0"},
		{in: "0202000A", want: "10.0.2.2"},
		{in: "0002000A", want: "10.0.2.0"},
		{in: "0100007F", want: "127.0.0.1"},
		{in: "00FFFFFF", want: "255.255.255.0"},
		{in: "ZZZZ0000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := hexLEToAddr(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("hexLEToAddr(%q) expected error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("hexLEToAddr(%q): %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Errorf("hexLEToAddr(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestDefaultRoute(t *testing.T) {
	tests := []struct {
		name   string
		routes []Route
		wantOK bool
	}{
		{
			name: "present and up",
			routes: []Route{
				{Interface: "eth0", Destination: netip.IPv4Unspecified(), Gateway: netip.MustParseAddr("10.0.2.2"), Flags: 0x3},
			},
			wantOK: true,
		},
		{
			name: "present but down",
			routes: []Route{
				{Interface: "eth0", Destination: netip.IPv4Unspecified(), Gateway: netip.MustParseAddr("10.0.2.2"), Flags: 0x2},
			},
			wantOK: false,
		},
		{
			name: "only specific routes",
			routes: []Route{
				{Interface: "eth0", Destination: netip.MustParseAddr("10.0.2.0"), Gateway: netip.Addr{}, Flags: 0x1},
			},
			wantOK: false,
		},
		{
			name:   "empty",
			routes: nil,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, ok := DefaultRoute(tt.routes)
			if ok != tt.wantOK {
				t.Fatalf("DefaultRoute ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && route.Gateway == netip.IPv4Unspecified() {
				t.Error("expected a non-zero gateway")
			}
		})
	}
}

func TestParseRouteTableErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "bad destination", in: "eth0 ZZZZ 00000000 0003\n", want: "destination"},
		{name: "bad gateway", in: "eth0 00000000 ZZZZ 0003\n", want: "gateway"},
		{name: "bad flags", in: "eth0 00000000 00000000 NOPE\n", want: "flags"},
		{name: "short line", in: "eth0 00000000\n", want: "3 fields"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRouteTable(strings.NewReader(tt.in))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %v should contain %q", err, tt.want)
			}
		})
	}
}

func TestProcRouteTableReader(t *testing.T) {
	reader := &ProcRouteTableReader{
		ReadFile: func(_ string) ([]byte, error) {
			return []byte(sampleRouteTable), nil
		},
	}

	routes, err := reader.ReadRouteTable(context.Background())
	if err != nil {
		t.Fatalf("ReadRouteTable: %v", err)
	}
	if len(routes) != 3 {
		t.Errorf("expected 3 routes, got %d", len(routes))
	}
}

func TestProcRouteTableReaderReadError(t *testing.T) {
	reader := &ProcRouteTableReader{
		Path:     "/tmp/missing",
		ReadFile: func(string) ([]byte, error) { return nil, context.DeadlineExceeded },
	}
	_, err := reader.ReadRouteTable(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProcRouteTableReaderCanceled(t *testing.T) {
	reader := &ProcRouteTableReader{
		ReadFile: func(string) ([]byte, error) { return []byte(sampleRouteTable), nil },
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reader.ReadRouteTable(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestRoutePredicates(t *testing.T) {
	up := Route{Flags: 0x1}
	down := Route{Flags: 0x0}
	if !up.IsUp() {
		t.Error("flags 0x1 should be up")
	}
	if down.IsUp() {
		t.Error("flags 0x0 should not be up")
	}

	def := Route{Destination: netip.IPv4Unspecified()}
	if !def.IsDefault() {
		t.Error("0.0.0.0 destination should be default")
	}
	specific := Route{Destination: netip.MustParseAddr("10.0.2.0")}
	if specific.IsDefault() {
		t.Error("specific destination should not be default")
	}
}
