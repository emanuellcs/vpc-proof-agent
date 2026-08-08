package cache

import (
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/report"
)

func testData() report.Data {
	return report.Data{
		Agent: report.AgentInfo{Version: "1.0.0"},
	}
}

func TestCacheEmpty(t *testing.T) {
	c := New(5 * time.Minute)

	if _, ok := c.Get(); ok {
		t.Fatal("expected cache miss when empty")
	}
	if !c.GeneratedAt().IsZero() {
		t.Error("GeneratedAt should be zero when empty")
	}
}

func TestCachePutGet(t *testing.T) {
	c := New(5 * time.Minute)
	data := testData()
	c.Put(&data)

	got, ok := c.Get()
	if !ok {
		t.Fatal("expected cache hit after Put")
	}
	if got.Agent.Version != data.Agent.Version {
		t.Errorf("cached data differs: %+v", got)
	}
	if c.GeneratedAt().IsZero() {
		t.Error("GeneratedAt should be set after Put")
	}
}

func TestCacheExpiry(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	c := New(5 * time.Minute)
	c.now = func() time.Time { return base }

	data := testData()
	c.Put(&data)

	if _, ok := c.Get(); !ok {
		t.Fatal("expected cache hit before expiry")
	}
	if c.GeneratedAt() != base {
		t.Errorf("GeneratedAt = %v, want %v", c.GeneratedAt(), base)
	}

	// Advance past the TTL.
	c.now = func() time.Time { return base.Add(6 * time.Minute) }
	if _, ok := c.Get(); ok {
		t.Fatal("expected cache miss after expiry")
	}
}

func TestCacheInvalidate(t *testing.T) {
	c := New(5 * time.Minute)
	data := testData()
	c.Put(&data)

	if _, ok := c.Get(); !ok {
		t.Fatal("expected cache hit")
	}

	c.Invalidate()
	if _, ok := c.Get(); ok {
		t.Fatal("expected cache miss after Invalidate")
	}
}

func TestCacheZeroTTLExpiresImmediately(t *testing.T) {
	c := New(0)
	data := testData()
	c.Put(&data)

	if _, ok := c.Get(); ok {
		t.Fatal("expected immediate expiry with zero TTL")
	}
}

func TestCacheReturnsStableGeneratedAt(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	c := New(5 * time.Minute)
	c.now = func() time.Time { return base }
	data := testData()
	c.Put(&data)

	if got := c.GeneratedAt(); got != base {
		t.Errorf("GeneratedAt = %v, want %v (stable across reads)", got, base)
	}
	if got := c.GeneratedAt(); got != base {
		t.Errorf("GeneratedAt changed between reads: %v", got)
	}
}
