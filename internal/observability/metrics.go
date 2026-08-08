package observability

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// durationBuckets are the histogram bucket boundaries in seconds, matching
// the standard Prometheus HTTP request duration buckets.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Metrics is a small, thread-safe registry of counters, a latency histogram,
// and per-probe status gauges that can be rendered in the Prometheus text
// exposition format. Every mutation is guarded by a single mutex because the
// registry is shared across concurrent HTTP handler goroutines.
type Metrics struct {
	mu sync.Mutex

	requestsTotal map[string]uint64 // key: method|path|status

	durationsCount   map[string]uint64   // key: method|path
	durationsSum     map[string]float64  // key: method|path
	durationsBuckets map[string][]uint64 // key: method|path -> bucket counts

	probeStatus map[string]string // probe id -> last status
}

// NewMetrics builds an empty registry.
func NewMetrics() *Metrics {
	return &Metrics{
		requestsTotal:    map[string]uint64{},
		durationsCount:   map[string]uint64{},
		durationsSum:     map[string]float64{},
		durationsBuckets: map[string][]uint64{},
		probeStatus:      map[string]string{},
	}
}

// IncRequest records one HTTP request for the given method, path, and status
// code.
func (m *Metrics) IncRequest(method, path, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestsTotal[requestKey(method, path, status)]++
}

// ObserveRequestDuration records the duration of one HTTP request.
func (m *Metrics) ObserveRequestDuration(method, path string, d time.Duration) {
	seconds := d.Seconds()
	m.mu.Lock()
	defer m.mu.Unlock()

	key := method + "|" + path
	m.durationsCount[key]++
	m.durationsSum[key] += seconds

	buckets, ok := m.durationsBuckets[key]
	if !ok {
		buckets = make([]uint64, len(durationBuckets))
	}
	for i, upper := range durationBuckets {
		if seconds <= upper {
			buckets[i]++
		}
	}
	m.durationsBuckets[key] = buckets
}

// SetProbeStatus records the last outcome of a probe. Setting a new status
// resets the previous status gauge for the probe to zero.
func (m *Metrics) SetProbeStatus(probeID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probeStatus[probeID] = status
}

// WritePrometheus renders the registry in the Prometheus text exposition
// format.
func (m *Metrics) WritePrometheus(w io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := fmt.Fprintf(w, "# HELP vpc_proof_http_requests_total Total HTTP requests by method, path, and status.\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "# TYPE vpc_proof_http_requests_total counter\n"); err != nil {
		return err
	}

	reqKeys := make([]string, 0, len(m.requestsTotal))
	for key := range m.requestsTotal {
		reqKeys = append(reqKeys, key)
	}
	sort.Strings(reqKeys)
	for _, key := range reqKeys {
		parts := strings.Split(key, "|")
		if _, err := fmt.Fprintf(w, "vpc_proof_http_requests_total{method=%q,path=%q,status=%q} %d\n",
			parts[0], parts[1], parts[2], m.requestsTotal[key]); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "# HELP vpc_proof_http_request_duration_seconds HTTP request latency.\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "# TYPE vpc_proof_http_request_duration_seconds histogram\n"); err != nil {
		return err
	}

	durKeys := make([]string, 0, len(m.durationsBuckets))
	for key := range m.durationsBuckets {
		durKeys = append(durKeys, key)
	}
	sort.Strings(durKeys)
	for _, key := range durKeys {
		parts := strings.SplitN(key, "|", 2)
		labels := fmt.Sprintf("method=%q,path=%q", parts[0], parts[1])
		for i, upper := range durationBuckets {
			if _, err := fmt.Fprintf(w, "vpc_proof_http_request_duration_seconds_bucket{%s,le=%q} %d\n",
				labels, formatLe(upper), m.durationsBuckets[key][i]); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "vpc_proof_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n",
			labels, m.durationsCount[key]); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "vpc_proof_http_request_duration_seconds_sum{%s} %g\n", labels, m.durationsSum[key]); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "vpc_proof_http_request_duration_seconds_count{%s} %d\n", labels, m.durationsCount[key]); err != nil {
			return err
		}
	}

	if len(m.probeStatus) > 0 {
		if _, err := fmt.Fprintf(w, "# HELP vpc_proof_probe_status Last probe outcome.\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "# TYPE vpc_proof_probe_status gauge\n"); err != nil {
			return err
		}
		probeIDs := make([]string, 0, len(m.probeStatus))
		for id := range m.probeStatus {
			probeIDs = append(probeIDs, id)
		}
		sort.Strings(probeIDs)
		for _, id := range probeIDs {
			status := m.probeStatus[id]
			if _, err := fmt.Fprintf(w, "vpc_proof_probe_status{probe=%q,status=%q} 1\n", id, status); err != nil {
				return err
			}
		}
	}

	return nil
}

// requestKey builds the map key for a request counter.
func requestKey(method, path, status string) string {
	return method + "|" + path + "|" + status
}

// formatLe formats a bucket upper bound, trimming trailing zeros.
func formatLe(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%g", v), "0"), ".")
}
