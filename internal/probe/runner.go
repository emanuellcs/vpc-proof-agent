package probe

import (
	"context"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

// Option configures a Runner.
type Option func(*Runner)

// WithLogger attaches a structured logger used to report probe outcomes.
func WithLogger(logger *observability.Logger) Option {
	return func(r *Runner) { r.logger = logger }
}

// WithProbeTimeout enforces a per-probe execution deadline.
func WithProbeTimeout(timeout time.Duration) Option {
	return func(r *Runner) { r.probeTimeout = timeout }
}

// WithGlobalTimeout bounds the whole run, wrapping the caller's context.
func WithGlobalTimeout(timeout time.Duration) Option {
	return func(r *Runner) { r.globalTimeout = timeout }
}

// Runner executes a set of probes and aggregates their results.
type Runner struct {
	probes        []Probe
	logger        *observability.Logger
	probeTimeout  time.Duration
	globalTimeout time.Duration
}

// NewRunner builds a Runner for the given probes.
func NewRunner(probes []Probe, opts ...Option) *Runner {
	runner := &Runner{probes: probes}
	for _, opt := range opts {
		opt(runner)
	}
	return runner
}

// Run executes every probe in order and returns an aggregated report.
//
// A global timeout wraps the caller's context when configured; each probe is
// additionally bounded by the per-probe timeout. A probe that outlives its
// deadline is reported as a warning with a "timed out" message.
func (r *Runner) Run(ctx context.Context) Report {
	if r.globalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.globalTimeout)
		defer cancel()
	}

	report := Report{StartedAt: time.Now().UTC()}
	for _, p := range r.probes {
		result := r.runOne(ctx, p)
		report.Results = append(report.Results, result)
		r.logResult(&result)
	}

	report.Duration = time.Since(report.StartedAt)
	report.Status = OverallStatus(report.Results)
	return report
}

// runOne executes a single probe with an optional per-probe deadline.
func (r *Runner) runOne(ctx context.Context, p Probe) Result {
	runCtx := ctx
	if r.probeTimeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, r.probeTimeout)
		defer cancel()
	}

	start := time.Now()
	result := p.Execute(runCtx)
	result.Duration = time.Since(start)
	result.ID = p.ID()

	if err := runCtx.Err(); err != nil {
		result.Status = StatusWarn
		result.Message = "probe timed out (context deadline exceeded)"
	}
	return result
}

// logResult reports a probe outcome to the configured logger, if any.
func (r *Runner) logResult(result *Result) {
	if r.logger == nil {
		return
	}
	fields := []observability.Field{
		observability.Component("probe"),
		observability.Str("probe", result.ID),
		observability.Str("status", result.Status.String()),
		observability.Duration("duration", result.Duration),
	}
	switch result.Status {
	case StatusPass:
		r.logger.Info("probe completed", fields...)
	case StatusWarn:
		r.logger.Warn("probe completed with warnings", fields...)
	case StatusSkip:
		r.logger.Info("probe skipped", fields...)
	default:
		r.logger.Error("probe failed", fields...)
	}
}
