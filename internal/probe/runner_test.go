package probe

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

type staticProbe struct {
	id     string
	status Status
}

func (s staticProbe) ID() string { return s.id }

func (s staticProbe) Execute(context.Context) Result {
	return Result{ID: s.id, Status: s.status, Message: s.id}
}

type blockingProbe struct {
	id string
}

func (b blockingProbe) ID() string { return b.id }

func (b blockingProbe) Execute(ctx context.Context) Result {
	<-ctx.Done()
	return Result{ID: b.id, Status: StatusPass, Message: "returned after cancellation"}
}

func TestRunnerAggregatesResults(t *testing.T) {
	runner := NewRunner([]Probe{
		staticProbe{id: "pass_probe", status: StatusPass},
		staticProbe{id: "warn_probe", status: StatusWarn},
		staticProbe{id: "fail_probe", status: StatusFail},
	})

	report := runner.Run(context.Background())

	if len(report.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(report.Results))
	}
	if report.Status != StatusFail {
		t.Errorf("overall status = %s, want fail", report.Status)
	}
	if report.Duration <= 0 {
		t.Errorf("report duration should be positive, got %s", report.Duration)
	}
	if report.StartedAt.IsZero() {
		t.Error("report started_at should be set")
	}
}

func TestRunnerResultLookup(t *testing.T) {
	runner := NewRunner([]Probe{staticProbe{id: "only", status: StatusPass}})
	report := runner.Run(context.Background())

	result, ok := report.Result("only")
	if !ok || result.Status != StatusPass {
		t.Errorf("Result(only) = %v, %v; want found pass", result, ok)
	}

	if _, ok := report.Result("missing"); ok {
		t.Error("Result(missing) should not be found")
	}
}

func TestRunnerEnforcesProbeTimeout(t *testing.T) {
	runner := NewRunner(
		[]Probe{blockingProbe{id: "slow"}},
		WithProbeTimeout(30*time.Millisecond),
	)

	report := runner.Run(context.Background())

	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	result := report.Results[0]
	if result.Status != StatusWarn {
		t.Errorf("status = %s, want warn after timeout", result.Status)
	}
	if !strings.Contains(result.Message, "timed out") {
		t.Errorf("message should mention timeout, got %q", result.Message)
	}
}

func TestRunnerEnforcesGlobalTimeout(t *testing.T) {
	runner := NewRunner(
		[]Probe{staticProbe{id: "fast", status: StatusPass}, blockingProbe{id: "slow"}},
		WithGlobalTimeout(30*time.Millisecond),
	)

	report := runner.Run(context.Background())

	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}
	fast, _ := report.Result("fast")
	if fast.Status != StatusPass {
		t.Errorf("fast probe status = %s, want pass", fast.Status)
	}
	slow, _ := report.Result("slow")
	if slow.Status != StatusWarn {
		t.Errorf("slow probe status = %s, want warn after global timeout", slow.Status)
	}
}

func TestRunnerEmptyProbes(t *testing.T) {
	runner := NewRunner(nil)
	report := runner.Run(context.Background())

	if len(report.Results) != 0 {
		t.Errorf("expected no results, got %d", len(report.Results))
	}
	if report.Status != StatusPass {
		t.Errorf("empty report status = %s, want pass", report.Status)
	}
}

func TestRunnerNilLogger(t *testing.T) {
	runner := NewRunner([]Probe{staticProbe{id: "x", status: StatusFail}})
	report := runner.Run(context.Background())
	if report.Status != StatusFail {
		t.Errorf("status = %s, want fail", report.Status)
	}
}

func TestRunnerWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger, err := observability.New("info", "json", &buf)
	if err != nil {
		t.Fatalf("observability.New: %v", err)
	}
	t.Cleanup(func() { _ = logger.Sync() })

	runner := NewRunner(
		[]Probe{staticProbe{id: "vpc_ownership", status: StatusFail}},
		WithLogger(logger),
	)

	runner.Run(context.Background())

	out := buf.String()
	if !strings.Contains(out, `"probe":"vpc_ownership"`) {
		t.Errorf("logger output should contain the probe id, got %s", out)
	}
	if !strings.Contains(out, `"status":"fail"`) {
		t.Errorf("logger output should contain the status, got %s", out)
	}
}

func TestOverallStatus(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    Status
	}{
		{name: "all pass", results: []Result{{Status: StatusPass}, {Status: StatusPass}}, want: StatusPass},
		{name: "warn over pass", results: []Result{{Status: StatusPass}, {Status: StatusWarn}}, want: StatusWarn},
		{name: "fail dominates warn", results: []Result{{Status: StatusWarn}, {Status: StatusFail}}, want: StatusFail},
		{name: "fail dominates pass", results: []Result{{Status: StatusPass}, {Status: StatusFail}}, want: StatusFail},
		{name: "all skip", results: []Result{{Status: StatusSkip}, {Status: StatusSkip}}, want: StatusSkip},
		{name: "skip and pass", results: []Result{{Status: StatusSkip}, {Status: StatusPass}}, want: StatusPass},
		{name: "empty", results: nil, want: StatusPass},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OverallStatus(tt.results); got != tt.want {
				t.Errorf("OverallStatus = %s, want %s", got, tt.want)
			}
		})
	}
}
