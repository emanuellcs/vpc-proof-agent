package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fixedNow is a fixed local clock used to make skew tests deterministic.
var fixedNow = time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

// dateServer returns a server whose Date header is now plus offset. An empty
// date omits the header entirely.
func dateServer(t *testing.T, offset time.Duration, date string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if date != "" {
			w.Header().Set("Date", date)
		} else if offset != 0 {
			w.Header().Set("Date", fixedNow.Add(offset).Format(http.TimeFormat))
		} else {
			w.Header().Set("Date", fixedNow.Format(http.TimeFormat))
		}
		w.WriteHeader(status)
	}))
}

func TestClockSkewProbePass(t *testing.T) {
	server := dateServer(t, 0, "", http.StatusOK)
	t.Cleanup(server.Close)

	p := NewClockSkewProbe(server.Client(), server.URL, func() time.Time { return fixedNow }, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusPass {
		t.Fatalf("status = %s, want pass: %s", result.Status, result.Message)
	}
	if result.Details["skew"] == "" {
		t.Error("skew detail missing")
	}
}

func TestClockSkewProbeNoFalsePositiveWithinGranularity(t *testing.T) {
	// A server 1.5s ahead is within Date-header granularity and nominal
	// network latency, so it must not be flagged.
	server := dateServer(t, 1500*time.Millisecond, "", http.StatusOK)
	t.Cleanup(server.Close)

	p := NewClockSkewProbe(server.Client(), server.URL, func() time.Time { return fixedNow }, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusPass {
		t.Fatalf("status = %s, want pass (granularity tolerance): %s", result.Status, result.Message)
	}
}

func TestClockSkewProbeWarn(t *testing.T) {
	server := dateServer(t, 5*time.Second, "", http.StatusOK)
	t.Cleanup(server.Close)

	p := NewClockSkewProbe(server.Client(), server.URL, func() time.Time { return fixedNow }, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusWarn {
		t.Fatalf("status = %s, want warn: %s", result.Status, result.Message)
	}
	if result.Hint != clockSkewHint {
		t.Errorf("hint = %q, want clock skew hint", result.Hint)
	}
}

func TestClockSkewProbeFail(t *testing.T) {
	server := dateServer(t, 15*time.Second, "", http.StatusOK)
	t.Cleanup(server.Close)

	p := NewClockSkewProbe(server.Client(), server.URL, func() time.Time { return fixedNow }, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusFail {
		t.Fatalf("status = %s, want fail: %s", result.Status, result.Message)
	}
	if result.Hint != clockSkewHint {
		t.Errorf("hint = %q, want clock skew hint", result.Hint)
	}
}

func TestClockSkewProbeSkewInThePast(t *testing.T) {
	// Server behind the local clock by 20s must also fail (skew is absolute).
	server := dateServer(t, -20*time.Second, "", http.StatusOK)
	t.Cleanup(server.Close)

	p := NewClockSkewProbe(server.Client(), server.URL, func() time.Time { return fixedNow }, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusFail {
		t.Fatalf("status = %s, want fail for past skew", result.Status)
	}
}

func TestClockSkewProbeMissingDateHeader(t *testing.T) {
	server := dateServer(t, 0, "", http.StatusOK)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Del("Date")
		w.WriteHeader(http.StatusOK)
	})
	t.Cleanup(server.Close)

	p := NewClockSkewProbe(server.Client(), server.URL, func() time.Time { return fixedNow }, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusFail {
		t.Fatalf("status = %s, want fail on missing Date header", result.Status)
	}
}

func TestClockSkewProbeBadDate(t *testing.T) {
	server := dateServer(t, 0, "not-a-date", http.StatusOK)
	t.Cleanup(server.Close)

	p := NewClockSkewProbe(server.Client(), server.URL, func() time.Time { return fixedNow }, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusFail {
		t.Fatalf("status = %s, want fail on bad Date header", result.Status)
	}
}

func TestClockSkewProbeNon2xx(t *testing.T) {
	server := dateServer(t, 0, "", http.StatusServiceUnavailable)
	t.Cleanup(server.Close)

	p := NewClockSkewProbe(server.Client(), server.URL, func() time.Time { return fixedNow }, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusFail {
		t.Fatalf("status = %s, want fail on non-2xx", result.Status)
	}
}

func TestClockSkewProbeConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	p := NewClockSkewProbe(&http.Client{Timeout: time.Second}, url, func() time.Time { return fixedNow }, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusFail {
		t.Fatalf("status = %s, want fail on connection error", result.Status)
	}
}
