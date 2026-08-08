package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

func TestStatusJSONRoundTrip(t *testing.T) {
	raw, err := json.Marshal(StatusPass)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `"pass"` {
		t.Errorf("marshal pass = %s, want %q", raw, `"pass"`)
	}

	var s Status
	if err := json.Unmarshal([]byte(`"warn"`), &s); err != nil {
		t.Fatalf("unmarshal warn: %v", err)
	}
	if s != StatusWarn {
		t.Errorf("unmarshaled = %d, want warn", s)
	}

	if err := json.Unmarshal([]byte(`"bogus"`), &s); err == nil {
		t.Error("expected an error for an invalid status name")
	}
	if err := json.Unmarshal([]byte(`123`), &s); err == nil {
		t.Error("expected an error for a non-string status")
	}
}

func TestStatusStringUnknown(t *testing.T) {
	if got := Status(99).String(); got != "unknown" {
		t.Errorf("Status(99).String() = %q, want unknown", got)
	}
}

func TestRunnerLoggerAllStatuses(t *testing.T) {
	var buf bytes.Buffer
	logger, err := observability.New("info", "json", &buf)
	if err != nil {
		t.Fatalf("observability.New: %v", err)
	}
	t.Cleanup(func() { _ = logger.Sync() })

	runner := NewRunner([]Probe{
		staticProbe{id: "a", status: StatusPass},
		staticProbe{id: "b", status: StatusWarn},
		staticProbe{id: "c", status: StatusFail},
		staticProbe{id: "d", status: StatusSkip},
	}, WithLogger(logger))

	runner.Run(context.Background())

	out := buf.String()
	for _, want := range []string{
		`"probe":"a"`, `"status":"pass"`,
		`"probe":"b"`, `"status":"warn"`,
		`"probe":"c"`, `"status":"fail"`,
		`"probe":"d"`, `"status":"skip"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("logger output missing %q", want)
		}
	}
}

func TestNewClockSkewProbeDefaultClock(t *testing.T) {
	// A server whose Date header reflects the real current time.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	p := NewClockSkewProbe(server.Client(), server.URL, nil, nil)
	result := p.Execute(context.Background())
	if result.Status != StatusPass {
		t.Fatalf("status = %s, want pass with the default clock", result.Status)
	}
}

func TestSystemResourcesProbeDefaultReader(t *testing.T) {
	p := NewSystemResourcesProbe(nil, nil)
	result := p.Execute(context.Background())
	if result.Status == StatusFail {
		t.Fatalf("status = %s, want pass/warn with the host /proc", result.Status)
	}
}

func TestInternetHTTPSProbeWithTimeout(t *testing.T) {
	server := newEchoServer(t, "203.0.113.7", http.StatusOK)

	p := NewInternetHTTPSProbe(server.Client(), server.URL, 0, time.Second, nil)
	result := p.Execute(context.Background())
	if result.Status != StatusPass {
		t.Fatalf("status = %s, want pass", result.Status)
	}
}
