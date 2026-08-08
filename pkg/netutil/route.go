package netutil

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// flagUp is the RTF_UP flag of a /proc/net/route entry.
const flagUp = 0x0001

// Route is a parsed entry of a /proc/net/route table.
type Route struct {
	// Interface is the network interface name carrying the route.
	Interface string
	// Destination is the route's destination network address.
	Destination netip.Addr
	// Gateway is the next-hop gateway address (0.0.0.0 for a direct route).
	Gateway netip.Addr
	// Flags holds the raw route flags (RTF_UP = 0x0001, RTF_GATEWAY = 0x0002).
	Flags uint32
}

// IsUp reports whether the route is active (RTF_UP set).
func (r Route) IsUp() bool {
	return r.Flags&flagUp != 0
}

// IsDefault reports whether the route covers 0.0.0.0/0.
func (r Route) IsDefault() bool {
	return r.Destination == netip.IPv4Unspecified()
}

// ParseRouteTable parses /proc/net/route content into structured routes.
// The header line and empty lines are skipped; malformed data lines produce
// a descriptive error.
func ParseRouteTable(r io.Reader) ([]Route, error) {
	scanner := bufio.NewScanner(r)
	var routes []Route

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		text := scanner.Text()
		if text == "" || strings.HasPrefix(text, "Iface") {
			continue
		}
		route, err := parseRouteLine(text)
		if err != nil {
			return nil, fmt.Errorf("route table line %d: %w", lineNo, err)
		}
		routes = append(routes, route)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read route table: %w", err)
	}
	return routes, nil
}

// parseRouteLine parses a single data line of a route table.
func parseRouteLine(line string) (Route, error) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return Route{}, fmt.Errorf("expected at least 3 fields, got %d", len(fields))
	}

	destination, err := hexLEToAddr(fields[1])
	if err != nil {
		return Route{}, fmt.Errorf("destination %q: %w", fields[1], err)
	}
	gateway, err := hexLEToAddr(fields[2])
	if err != nil {
		return Route{}, fmt.Errorf("gateway %q: %w", fields[2], err)
	}

	var flags uint32
	if len(fields) > 3 {
		parsed, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil {
			return Route{}, fmt.Errorf("flags %q: %w", fields[3], err)
		}
		flags = uint32(parsed)
	}

	return Route{
		Interface:   fields[0],
		Destination: destination,
		Gateway:     gateway,
		Flags:       flags,
	}, nil
}

// hexLEToAddr decodes a 32-bit little-endian hexadecimal field (as used by
// /proc/net/route) into a netip.Addr. For example "0202000A" -> 10.0.2.2.
func hexLEToAddr(s string) (netip.Addr, error) {
	value, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return netip.Addr{}, err
	}
	// #nosec G115 -- each byte is derived from an explicit 8-bit shift of a
	// value parsed with bit size 32, so no overflow is possible.
	return netip.AddrFrom4([4]byte{byte(value), byte(value >> 8), byte(value >> 16), byte(value >> 24)}), nil
}

// DefaultRoute returns the active default route (destination 0.0.0.0 and
// RTF_UP set), if one exists.
func DefaultRoute(routes []Route) (Route, bool) {
	for _, r := range routes {
		if r.IsDefault() && r.IsUp() {
			return r, true
		}
	}
	return Route{}, false
}

// RouteTableReader abstracts access to the operating system route table so it
// can be fed from injected data in tests.
type RouteTableReader interface {
	// ReadRouteTable returns the parsed routing table.
	ReadRouteTable(ctx context.Context) ([]Route, error)
}

// ProcRouteTableReader reads the route table from a /proc/net/route-style
// file. The path and the file-reading function are injectable for testing.
type ProcRouteTableReader struct {
	// Path is the route table file. Defaults to /proc/net/route.
	Path string
	// ReadFile reads a file by path. Defaults to os.ReadFile.
	ReadFile func(string) ([]byte, error)
}

// NewProcRouteTableReader builds a reader for /proc/net/route.
func NewProcRouteTableReader() *ProcRouteTableReader {
	return &ProcRouteTableReader{
		Path:     "/proc/net/route",
		ReadFile: os.ReadFile,
	}
}

// ReadRouteTable reads and parses the route table, honoring context
// cancellation at the read boundaries.
func (r *ProcRouteTableReader) ReadRouteTable(ctx context.Context) ([]Route, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := r.ReadFile(r.Path)
	if err != nil {
		return nil, fmt.Errorf("read route table %q: %w", r.Path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ParseRouteTable(bytes.NewReader(data))
}
