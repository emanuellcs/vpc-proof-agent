package diagnostic

import (
	"strings"
	"testing"

	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

// reportWith builds a report from a probe ID to status map.
func reportWith(statuses map[string]probe.Status) probe.Report {
	var results []probe.Result
	for id, status := range statuses {
		results = append(results, probe.Result{ID: id, Status: status})
	}
	return probe.Report{Results: results}
}

func hintMessages(hints []Hint) []string {
	messages := make([]string, 0, len(hints))
	for _, h := range hints {
		messages = append(messages, h.Message)
	}
	return messages
}

func containsHint(hints []Hint, fragment string) bool {
	for _, h := range hints {
		if strings.Contains(h.Message, fragment) {
			return true
		}
	}
	return false
}

func TestAnalyzeAllPass(t *testing.T) {
	engine := New()
	hints := engine.Analyze(reportWith(map[string]probe.Status{
		probe.MetadataProbeID:            probe.StatusPass,
		probe.VPCOwnershipProbeID:        probe.StatusPass,
		probe.SubnetOwnershipProbeID:     probe.StatusPass,
		probe.DefaultRouteProbeID:        probe.StatusPass,
		probe.DNSProbeID:                 probe.StatusPass,
		probe.InternetHTTPSProbeID:       probe.StatusPass,
		probe.PublicIPConsistencyProbeID: probe.StatusPass,
	}))

	if len(hints) != 0 {
		t.Fatalf("expected no hints for an all-pass report, got %v", hintMessages(hints))
	}
}

func TestAnalyzeEmptyReport(t *testing.T) {
	if hints := New().Analyze(probe.Report{}); len(hints) != 0 {
		t.Fatalf("expected no hints for an empty report, got %v", hintMessages(hints))
	}
}

func TestAnalyzeRuleMatrix(t *testing.T) {
	tests := []struct {
		name     string
		statuses map[string]probe.Status
		want     string
		ruleID   string
	}{
		{
			name: "https fails but dns passes",
			statuses: map[string]probe.Status{
				probe.InternetHTTPSProbeID: probe.StatusFail,
				probe.DNSProbeID:           probe.StatusPass,
			},
			want:   "Internet Gateway",
			ruleID: "igw-route-table",
		},
		{
			name:     "subnet ownership fails",
			statuses: map[string]probe.Status{probe.SubnetOwnershipProbeID: probe.StatusFail},
			want:     "correct Subnet (10.0.1.0/24)",
			ruleID:   "subnet-placement",
		},
		{
			name:     "public ip consistency fails",
			statuses: map[string]probe.Status{probe.PublicIPConsistencyProbeID: probe.StatusFail},
			want:     "Auto-assign public IP",
			ruleID:   "public-ip-assignment",
		},
		{
			name:     "default route fails",
			statuses: map[string]probe.Status{probe.DefaultRouteProbeID: probe.StatusFail},
			want:     "0.0.0.0/0 route to the Internet Gateway",
			ruleID:   "default-route",
		},
		{
			name:     "vpc ownership fails",
			statuses: map[string]probe.Status{probe.VPCOwnershipProbeID: probe.StatusFail},
			want:     "10.0.0.0/16",
			ruleID:   "vpc-mismatch",
		},
		{
			name:     "dns fails",
			statuses: map[string]probe.Status{probe.DNSProbeID: probe.StatusFail},
			want:     "VPC DNS settings",
			ruleID:   "dns-configuration",
		},
		{
			name:     "metadata fails",
			statuses: map[string]probe.Status{probe.MetadataProbeID: probe.StatusFail},
			want:     "IMDSv2",
			ruleID:   "imds-configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := New()
			hints := engine.Analyze(reportWith(tt.statuses))

			if !containsHint(hints, tt.want) {
				t.Fatalf("expected a hint containing %q, got %v", tt.want, hintMessages(hints))
			}
			for _, h := range hints {
				if strings.Contains(h.Message, tt.want) && h.RuleID != tt.ruleID {
					t.Errorf("hint %q produced by rule %q, want rule %q", h.Message, h.RuleID, tt.ruleID)
				}
			}
		})
	}
}

func TestAnalyzeIGWRuleRequiresDNSPass(t *testing.T) {
	// When BOTH internet_https and dns fail, the IGW rule (which requires
	// dns=pass) must not match; the DNS rule matches instead.
	statuses := map[string]probe.Status{
		probe.InternetHTTPSProbeID: probe.StatusFail,
		probe.DNSProbeID:           probe.StatusFail,
	}

	hints := New().Analyze(reportWith(statuses))

	if containsHint(hints, "Internet Gateway") {
		t.Errorf("IGW rule should not match when DNS also fails, got %v", hintMessages(hints))
	}
	if !containsHint(hints, "VPC DNS settings") {
		t.Errorf("DNS rule should match, got %v", hintMessages(hints))
	}
}

func TestAnalyzeMissingProbeDisablesRule(t *testing.T) {
	// The metadata probe result is absent, so the IMDS rule must not match.
	statuses := map[string]probe.Status{
		probe.DNSProbeID: probe.StatusFail,
	}

	hints := New().Analyze(reportWith(statuses))

	if containsHint(hints, "IMDSv2") {
		t.Errorf("IMDS rule should not match when the probe result is missing, got %v", hintMessages(hints))
	}
}

func TestAnalyzeStatusMismatchDisablesRule(t *testing.T) {
	// A warn status does not satisfy a rule that requires fail.
	statuses := map[string]probe.Status{
		probe.SubnetOwnershipProbeID: probe.StatusWarn,
	}

	hints := New().Analyze(reportWith(statuses))

	if containsHint(hints, "correct Subnet") {
		t.Errorf("subnet rule should not match on warn, got %v", hintMessages(hints))
	}
}

func TestAnalyzeMultipleRulesInOrder(t *testing.T) {
	statuses := map[string]probe.Status{
		probe.SubnetOwnershipProbeID:     probe.StatusFail,
		probe.PublicIPConsistencyProbeID: probe.StatusFail,
		probe.DNSProbeID:                 probe.StatusPass,
		probe.InternetHTTPSProbeID:       probe.StatusFail,
	}

	hints := New().Analyze(reportWith(statuses))

	// The IGW rule runs first, then subnet, then public-ip.
	var ruleOrder []string
	for _, h := range hints {
		ruleOrder = append(ruleOrder, h.RuleID)
	}

	if len(ruleOrder) != 3 {
		t.Fatalf("expected 3 hints, got %d: %v", len(ruleOrder), hintMessages(hints))
	}
	if ruleOrder[0] != "igw-route-table" || ruleOrder[1] != "subnet-placement" || ruleOrder[2] != "public-ip-assignment" {
		t.Errorf("unexpected rule order: %v", ruleOrder)
	}
}

func TestNewWithRules(t *testing.T) {
	custom := []Rule{
		{
			ID:       "custom",
			Requires: map[string]probe.Status{probe.DNSProbeID: probe.StatusFail},
			Severity: SeverityInfo,
			Hints:    []string{"custom hint"},
		},
	}

	hints := NewWithRules(custom).Analyze(reportWith(map[string]probe.Status{probe.DNSProbeID: probe.StatusFail}))
	if len(hints) != 1 || hints[0].Message != "custom hint" || hints[0].Severity != SeverityInfo {
		t.Fatalf("custom rule not honored: %v", hints)
	}
}

func TestRuleWithMultipleHints(t *testing.T) {
	custom := []Rule{
		{
			ID:       "multi",
			Requires: map[string]probe.Status{probe.DNSProbeID: probe.StatusFail},
			Severity: SeverityWarning,
			Hints:    []string{"first", "second"},
		},
	}

	hints := NewWithRules(custom).Analyze(reportWith(map[string]probe.Status{probe.DNSProbeID: probe.StatusFail}))
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints, got %d", len(hints))
	}
	if hints[0].RuleID != "multi" || hints[1].RuleID != "multi" {
		t.Errorf("both hints should carry the rule id: %+v", hints)
	}
}

func TestHintSeverities(t *testing.T) {
	// The IGW and subnet rules are critical; public-ip is a warning.
	statuses := map[string]probe.Status{
		probe.InternetHTTPSProbeID:       probe.StatusFail,
		probe.DNSProbeID:                 probe.StatusPass,
		probe.PublicIPConsistencyProbeID: probe.StatusFail,
	}

	hints := New().Analyze(reportWith(statuses))
	severities := map[string]Severity{}
	for _, h := range hints {
		severities[h.RuleID] = h.Severity
	}

	if severities["igw-route-table"] != SeverityCritical {
		t.Errorf("igw rule severity = %q, want critical", severities["igw-route-table"])
	}
	if severities["public-ip-assignment"] != SeverityWarning {
		t.Errorf("public-ip rule severity = %q, want warning", severities["public-ip-assignment"])
	}
}
