package observability

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMetricsIncRequest(t *testing.T) {
	m := NewMetrics()
	m.IncRequest("GET", "/healthz", "200")
	m.IncRequest("GET", "/healthz", "200")
	m.IncRequest("GET", "/api/v1/info", "200")

	var out strings.Builder
	if err := m.WritePrometheus(&out); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"# HELP vpc_proof_http_requests_total",
		"# TYPE vpc_proof_http_requests_total counter",
		`vpc_proof_http_requests_total{method="GET",path="/healthz",status="200"} 2`,
		`vpc_proof_http_requests_total{method="GET",path="/api/v1/info",status="200"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prometheus output missing %q:\n%s", want, text)
		}
	}
}

func TestMetricsObserveDuration(t *testing.T) {
	m := NewMetrics()
	m.ObserveRequestDuration("GET", "/api/v1/probe", 1500*time.Millisecond)
	m.ObserveRequestDuration("GET", "/api/v1/probe", 10*time.Millisecond)

	var out strings.Builder
	if err := m.WritePrometheus(&out); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, `vpc_proof_http_request_duration_seconds_count{method="GET",path="/api/v1/probe"} 2`) {
		t.Errorf("duration count missing:\n%s", text)
	}
	if !strings.Contains(text, `vpc_proof_http_request_duration_seconds_sum{method="GET",path="/api/v1/probe"} 1.51`) {
		t.Errorf("duration sum missing:\n%s", text)
	}
	if !strings.Contains(text, `vpc_proof_http_request_duration_seconds_bucket{method="GET",path="/api/v1/probe",le="0.005"} 0`) {
		t.Errorf("first bucket missing:\n%s", text)
	}
	if !strings.Contains(text, `vpc_proof_http_request_duration_seconds_bucket{method="GET",path="/api/v1/probe",le="2.5"} 2`) {
		t.Errorf("2.5 bucket should contain both samples:\n%s", text)
	}
	if !strings.Contains(text, `vpc_proof_http_request_duration_seconds_bucket{method="GET",path="/api/v1/probe",le="+Inf"} 2`) {
		t.Errorf("+Inf bucket missing:\n%s", text)
	}
}

func TestMetricsProbeStatus(t *testing.T) {
	m := NewMetrics()
	m.SetProbeStatus("dns", "pass")
	m.SetProbeStatus("metadata", "fail")

	var out strings.Builder
	if err := m.WritePrometheus(&out); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, `vpc_proof_probe_status{probe="dns",status="pass"} 1`) {
		t.Errorf("probe status gauge missing:\n%s", text)
	}
	if !strings.Contains(text, `vpc_proof_probe_status{probe="metadata",status="fail"} 1`) {
		t.Errorf("probe status gauge missing:\n%s", text)
	}
}

func TestMetricsProbeStatusReset(t *testing.T) {
	m := NewMetrics()
	m.SetProbeStatus("dns", "pass")
	m.SetProbeStatus("dns", "fail")

	var out strings.Builder
	if err := m.WritePrometheus(&out); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	text := out.String()
	if strings.Contains(text, `status="pass"`) {
		t.Errorf("previous status gauge should be reset:\n%s", text)
	}
	if !strings.Contains(text, `status="fail"`) {
		t.Errorf("latest status gauge missing:\n%s", text)
	}
}

func TestMetricsEmpty(t *testing.T) {
	m := NewMetrics()
	var out strings.Builder
	if err := m.WritePrometheus(&out); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	if !strings.Contains(out.String(), "# TYPE vpc_proof_http_requests_total counter") {
		t.Error("empty registry should still emit the counter type")
	}
}

func TestMetricsDeterministicOrder(t *testing.T) {
	m := NewMetrics()
	m.IncRequest("POST", "/b", "200")
	m.IncRequest("GET", "/a", "200")
	m.IncRequest("GET", "/a", "500")

	first := render(t, m)
	second := render(t, m)
	if first != second {
		t.Errorf("prometheus output is not deterministic:\n%s\n---\n%s", first, second)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestMetricsWriteError(t *testing.T) {
	m := NewMetrics()
	m.IncRequest("GET", "/x", "200")

	if err := m.WritePrometheus(failWriter{}); err == nil {
		t.Fatal("expected write error, got nil")
	}
}

// countingFailWriter fails on the Nth write.
type countingFailWriter struct {
	remaining int
}

func (w *countingFailWriter) Write(b []byte) (int, error) {
	if w.remaining == 0 {
		return 0, errors.New("write failed")
	}
	w.remaining--
	return len(b), nil
}

func TestMetricsWriteErrorsAcrossBranches(t *testing.T) {
	m := NewMetrics()
	m.IncRequest("GET", "/a", "200")
	m.IncRequest("GET", "/b", "200")
	m.ObserveRequestDuration("GET", "/a", 1500*time.Millisecond)
	m.SetProbeStatus("dns", "pass")

	sawError := false
	for remaining := range 40 {
		if err := m.WritePrometheus(&countingFailWriter{remaining: remaining}); err != nil {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("expected at least one write error across branches")
	}
}

func render(t *testing.T, m *Metrics) string {
	t.Helper()
	var out strings.Builder
	if err := m.WritePrometheus(&out); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	return out.String()
}
