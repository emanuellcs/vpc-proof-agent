package diagnostic

import (
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

// Severity classifies a troubleshooting hint.
type Severity string

// Hint severities.
const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Hint is an actionable, AWS-specific troubleshooting suggestion.
type Hint struct {
	// RuleID identifies the rule that produced the hint.
	RuleID string `json:"rule_id"`
	// Severity is the hint's importance.
	Severity Severity `json:"severity"`
	// Message is the human-readable guidance.
	Message string `json:"message"`
}

// Rule maps a combination of probe outcomes to one or more hints.
//
// A rule matches when every probe outcome in Requires is present in the
// report with exactly the required status.
type Rule struct {
	// ID is a stable rule identifier.
	ID string `json:"id"`
	// Requires maps a probe ID to the status it must have for the rule to
	// match. Adding a rule is simply appending an entry to the rule set.
	Requires map[string]probe.Status `json:"requires"`
	// Severity applies to all hints produced by the rule.
	Severity Severity `json:"severity"`
	// Hints are the messages produced when the rule matches.
	Hints []string `json:"hints"`
}

// Engine evaluates an ordered set of rules against a probe report.
type Engine struct {
	rules []Rule
}

// New builds an Engine with the built-in rule matrix.
func New() *Engine {
	return &Engine{rules: defaultRules()}
}

// NewWithRules builds an Engine with a custom rule set, useful for tests and
// for applications that need a tailored matrix.
func NewWithRules(rules []Rule) *Engine {
	return &Engine{rules: rules}
}

// Analyze evaluates every rule in order against the report and returns the
// matching hints. Each matching rule contributes its hints once; a rule whose
// required probe is missing from the report never matches.
func (e *Engine) Analyze(report probe.Report) []Hint {
	var hints []Hint
	for _, rule := range e.rules {
		if !ruleMatches(rule, report) {
			continue
		}
		for _, message := range rule.Hints {
			hints = append(hints, Hint{RuleID: rule.ID, Severity: rule.Severity, Message: message})
		}
	}
	return hints
}

// ruleMatches reports whether the report satisfies every required probe
// outcome of the rule.
func ruleMatches(rule Rule, report probe.Report) bool {
	for id, required := range rule.Requires {
		result, ok := report.Result(id)
		if !ok {
			return false
		}
		if result.Status != required {
			return false
		}
	}
	return true
}
