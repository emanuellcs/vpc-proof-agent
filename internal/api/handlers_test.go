package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// startedAtOf extracts the started_at field from a probe report JSON body.
func startedAtOf(t *testing.T, body string) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("decode probe report: %v", err)
	}
	started, _ := doc["started_at"].(string)
	return started
}

func TestProbeCacheIdentity(t *testing.T) {
	_, probeCache, server := newTestServer(t, nil)

	first := readBody(t, get(t, server.URL+"/api/v1/probe"))
	second := readBody(t, get(t, server.URL+"/api/v1/probe"))

	if first != second {
		t.Error("consecutive requests within the TTL must return identical cached reports")
	}
	if startedAtOf(t, first) != startedAtOf(t, second) {
		t.Error("started_at must be identical across cached requests")
	}
	if probeCache.GeneratedAt().IsZero() {
		t.Error("cache GeneratedAt should be set after a probe run")
	}
}

func TestProbeForceRefresh(t *testing.T) {
	_, probeCache, server := newTestServer(t, nil)

	cached := readBody(t, get(t, server.URL+"/api/v1/probe"))
	cachedStarted := startedAtOf(t, cached)
	generatedBefore := probeCache.GeneratedAt()

	// A cached hit must not change the generation timestamp.
	_ = readBody(t, get(t, server.URL+"/api/v1/probe"))
	if !probeCache.GeneratedAt().Equal(generatedBefore) {
		t.Fatal("cached hit should not regenerate the report")
	}

	// A forced refresh must run a fresh probe execution.
	resp := doRequest(t, http.MethodGet, server.URL+"/api/v1/probe", map[string]string{
		forceRefreshHeader: "true",
	})
	refreshed := readBody(t, resp)
	refreshedStarted := startedAtOf(t, refreshed)

	if refreshedStarted == cachedStarted {
		t.Error("force refresh should produce a new probe run with a different started_at")
	}
	if probeCache.GeneratedAt().Equal(generatedBefore) {
		t.Error("force refresh should update the cache generation timestamp")
	}
}

func TestStatusUsesCache(t *testing.T) {
	_, probeCache, server := newTestServer(t, nil)

	// The status endpoint triggers a probe run and caches it.
	_ = readBody(t, get(t, server.URL+"/api/v1/status"))
	if probeCache.GeneratedAt().IsZero() {
		t.Fatal("status endpoint should populate the cache")
	}

	// A subsequent probe request must be served from that cache.
	body := readBody(t, get(t, server.URL+"/api/v1/probe"))
	if startedAtOf(t, body) == "" {
		t.Error("probe report should be present")
	}
}

func TestReportEndpointUsesCache(t *testing.T) {
	_, probeCache, server := newTestServer(t, nil)

	first := readBody(t, get(t, server.URL+"/api/v1/report?format=json"))
	second := readBody(t, get(t, server.URL+"/api/v1/report?format=json"))

	if first != second {
		t.Error("cached report requests must return identical output")
	}
	if probeCache.GeneratedAt().IsZero() {
		t.Error("report endpoint should populate the cache")
	}
}
