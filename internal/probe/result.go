package probe

import (
	"encoding/json"
	"fmt"
	"time"
)

// Status is the outcome of a probe execution.
type Status int

// Probe statuses.
const (
	StatusPass Status = iota
	StatusFail
	StatusWarn
	StatusSkip
)

// String returns the lowercase name of the status.
func (s Status) String() string {
	switch s {
	case StatusPass:
		return "pass"
	case StatusFail:
		return "fail"
	case StatusWarn:
		return "warn"
	case StatusSkip:
		return "skip"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes the status as its lowercase name, keeping JSON
// reports human-readable.
func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON decodes a status from its lowercase name.
func (s *Status) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	switch text {
	case "pass":
		*s = StatusPass
	case "fail":
		*s = StatusFail
	case "warn":
		*s = StatusWarn
	case "skip":
		*s = StatusSkip
	default:
		return fmt.Errorf("invalid probe status %q", text)
	}
	return nil
}

// Result is the outcome of a single probe execution.
type Result struct {
	// ID is the stable probe identifier (for example "vpc_ownership").
	ID string `json:"id"`
	// Name is a human-readable probe name.
	Name string `json:"name"`
	// Status is the probe outcome.
	Status Status `json:"status"`
	// Duration is how long the probe took.
	Duration time.Duration `json:"duration"`
	// Message summarizes the outcome in a single sentence.
	Message string `json:"message"`
	// Details holds technical details keyed by name.
	Details map[string]string `json:"details,omitempty"`
	// Hint is a troubleshooting hint attached on failure.
	Hint string `json:"hint,omitempty"`
}

// Report aggregates the results of a probe run.
type Report struct {
	// StartedAt is when the run began.
	StartedAt time.Time `json:"started_at"`
	// Duration is the total wall-clock time of the run.
	Duration time.Duration `json:"duration"`
	// Status is the aggregate outcome of all results.
	Status Status `json:"status"`
	// Results holds the individual probe results in execution order.
	Results []Result `json:"results"`
}

// Result returns the result with the given ID, if present.
func (r Report) Result(id string) (Result, bool) {
	for _, result := range r.Results {
		if result.ID == id {
			return result, true
		}
	}
	return Result{}, false
}

// OverallStatus computes the aggregate status of a set of results:
// any fail dominates, then any warn, then skip; otherwise pass.
func OverallStatus(results []Result) Status {
	hasFail := false
	hasWarn := false
	allSkip := len(results) > 0

	for _, result := range results {
		switch result.Status {
		case StatusFail:
			hasFail = true
		case StatusWarn:
			hasWarn = true
		}
		if result.Status != StatusSkip {
			allSkip = false
		}
	}

	switch {
	case hasFail:
		return StatusFail
	case hasWarn:
		return StatusWarn
	case allSkip:
		return StatusSkip
	default:
		return StatusPass
	}
}
