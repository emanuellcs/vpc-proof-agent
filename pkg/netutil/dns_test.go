package netutil

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

type fakeResolver struct {
	addrs []netip.Addr
	err   error
	host  string
}

func (f fakeResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.host != "" && host != f.host {
		return nil, errors.New("unexpected host queried")
	}
	return f.addrs, nil
}

func TestResolve(t *testing.T) {
	addrs := []netip.Addr{netip.MustParseAddr("203.0.113.10"), netip.MustParseAddr("203.0.113.11")}
	resolver := fakeResolver{addrs: addrs, host: "amazon.com"}

	got, err := Resolve(context.Background(), resolver, "amazon.com")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(got))
	}
	if got[0] != addrs[0] {
		t.Errorf("first address = %s, want %s", got[0], addrs[0])
	}
}

func TestResolvePropagatesError(t *testing.T) {
	resolver := fakeResolver{err: errors.New("no such host")}

	_, err := Resolve(context.Background(), resolver, "does-not-exist.invalid")
	if err == nil {
		t.Fatal("expected resolver error to propagate, got nil")
	}
}

func TestResolveWrapsHost(t *testing.T) {
	resolver := fakeResolver{err: errors.New("nope")}

	_, err := Resolve(context.Background(), resolver, "amazon.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
