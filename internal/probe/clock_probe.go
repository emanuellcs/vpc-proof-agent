package probe

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

// Clock skew thresholds.
//
// HTTP Date headers carry second-level precision and network transit adds
// latency, so clockSkewGranularityTolerance is subtracted from the measured
// skew before comparing against the thresholds. This avoids false positives
// caused purely by sub-second truncation or round-trip time.
const (
	// clockSkewPassThreshold is the maximum accepted skew.
	clockSkewPassThreshold = 2 * time.Second
	// clockSkewFailThreshold is the skew beyond which the probe fails.
	clockSkewFailThreshold = 10 * time.Second
	// clockSkewGranularityTolerance accounts for Date header truncation and
	// network latency.
	clockSkewGranularityTolerance = time.Second
)

// clockSkewHint is attached whenever the clock appears to drift.
const clockSkewHint = "Clock skew detected. Verify NTP configuration and ensure the instance is synchronized with Amazon Time Sync Service."

// ClockSkewProbe compares the remote server's Date header with the local
// clock, detecting drift that would break TLS and AWS API signatures.
type ClockSkewProbe struct {
	client *http.Client
	url    string
	now    func() time.Time
	logger *observability.Logger
}

// NewClockSkewProbe builds a probe that HEAD-requests url and compares the
// Date header against now. A nil now falls back to time.Now; tests inject a
// fixed clock for determinism.
func NewClockSkewProbe(client *http.Client, url string, now func() time.Time, logger *observability.Logger) *ClockSkewProbe {
	if now == nil {
		now = time.Now
	}
	return &ClockSkewProbe{client: client, url: url, now: now, logger: logger}
}

// ID returns the probe identifier.
func (p *ClockSkewProbe) ID() string {
	return ClockSkewProbeID
}

// Execute fetches the remote Date header and grades the local clock skew.
func (p *ClockSkewProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	result := Result{
		ID:      p.ID(),
		Name:    "Clock skew",
		Details: map[string]string{},
	}

	serverTime, err := p.fetchServerTime(ctx)
	if err != nil {
		result.Status = StatusFail
		result.Duration = time.Since(start)
		result.Message = fmt.Sprintf("could not determine the remote time: %v", err)
		result.Hint = clockSkewHint
		return result
	}

	localTime := p.now().UTC()
	measured := localTime.Sub(serverTime)
	if measured < 0 {
		measured = -measured
	}
	skew := measured - clockSkewGranularityTolerance
	if skew < 0 {
		skew = 0
	}

	result.Details["server_time"] = serverTime.Format(time.RFC3339)
	result.Details["local_time"] = localTime.Format(time.RFC3339)
	result.Details["skew"] = skew.String()

	result.Duration = time.Since(start)
	switch {
	case skew < clockSkewPassThreshold:
		result.Status = StatusPass
		result.Message = fmt.Sprintf("clock skew %s is within acceptable bounds", skew)
	case skew <= clockSkewFailThreshold:
		result.Status = StatusWarn
		result.Message = fmt.Sprintf("clock skew %s is slightly out of bounds", skew)
		result.Hint = clockSkewHint
	default:
		result.Status = StatusFail
		result.Message = fmt.Sprintf("clock skew %s is critical", skew)
		result.Hint = clockSkewHint
	}

	if p.logger != nil {
		p.logger.Debug("clock skew checked",
			observability.Component("probe"),
			observability.Str("skew", skew.String()),
		)
	}
	return result
}

// fetchServerTime performs a HEAD request and parses the Date header.
func (p *ClockSkewProbe) fetchServerTime(ctx context.Context) (time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, p.url, http.NoBody)
	if err != nil {
		return time.Time{}, fmt.Errorf("build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("request %s: %w", p.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return time.Time{}, fmt.Errorf("request %s returned status %d", p.url, resp.StatusCode)
	}

	date := resp.Header.Get("Date")
	if date == "" {
		return time.Time{}, fmt.Errorf("missing Date header")
	}
	parsed, err := http.ParseTime(date)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Date header %q: %w", date, err)
	}
	return parsed.UTC(), nil
}
