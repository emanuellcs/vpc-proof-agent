package report

import (
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/diagnostic"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

func TestBuild(t *testing.T) {
	startedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	pr := probe.Report{
		StartedAt: startedAt,
		Status:    probe.StatusWarn,
		Duration:  2500 * time.Millisecond,
		Results: []probe.Result{
			{ID: probe.MetadataProbeID, Status: probe.StatusPass, Details: map[string]string{
				"instance_id": "i-0123456789abcdef0", "private_ip": "10.0.1.42",
				"public_ip": "203.0.113.7", "availability_zone": "us-east-1a",
			}},
			{ID: probe.VPCOwnershipProbeID, Status: probe.StatusPass, Details: map[string]string{"vpc_cidr": "10.0.0.0/16"}},
			{ID: probe.SubnetOwnershipProbeID, Status: probe.StatusPass, Details: map[string]string{"subnet_cidr": "10.0.1.0/24"}},
			{ID: probe.DefaultRouteProbeID, Status: probe.StatusPass, Details: map[string]string{
				"gateway": "10.0.2.2", "interface": "eth0", "primary_ip": "10.0.1.42",
			}},
			{ID: probe.DNSProbeID, Status: probe.StatusPass, Details: map[string]string{"addresses": "203.0.113.10"}},
			{ID: probe.PublicIPConsistencyProbeID, Status: probe.StatusWarn},
			{ID: "extra_fail", Status: probe.StatusFail},
			{ID: "extra_skip", Status: probe.StatusSkip},
		},
	}
	hints := []diagnostic.Hint{{RuleID: "custom", Severity: diagnostic.SeverityWarning, Message: "custom hint"}}
	agent := AgentInfo{Name: "vpc-proof", Version: "1.0.0", Commit: "abc"}

	data := Build(pr, hints, &agent)

	if data.GeneratedAt != startedAt {
		t.Errorf("GeneratedAt = %v, want %v", data.GeneratedAt, startedAt)
	}
	if data.Agent.Version != "1.0.0" {
		t.Errorf("Agent.Version = %q, want 1.0.0", data.Agent.Version)
	}

	instance := data.Instance
	if instance.InstanceID != "i-0123456789abcdef0" || instance.PrivateIP != "10.0.1.42" ||
		instance.PublicIP != "203.0.113.7" || instance.AvailabilityZone != "us-east-1a" {
		t.Errorf("instance extraction incorrect: %+v", instance)
	}
	if instance.VpcCIDR != "10.0.0.0/16" || instance.SubnetCIDR != "10.0.1.0/24" {
		t.Errorf("instance CIDR extraction incorrect: %+v", instance)
	}

	network := data.Network
	if network.DefaultGateway != "10.0.2.2" || network.DefaultInterface != "eth0" ||
		network.PrimaryIP != "10.0.1.42" || network.DNSAddresses != "203.0.113.10" {
		t.Errorf("network extraction incorrect: %+v", network)
	}

	summary := data.Summary
	if summary.Total != 8 || summary.Passed != 5 || summary.Warned != 1 || summary.Failed != 1 || summary.Skipped != 1 {
		t.Errorf("summary counts incorrect: %+v", summary)
	}
	if summary.Status != "warn" {
		t.Errorf("summary status = %q, want warn", summary.Status)
	}
	if summary.Duration != 2500*time.Millisecond {
		t.Errorf("summary duration = %s", summary.Duration)
	}

	if len(data.Diagnostics) != 1 || data.Diagnostics[0].Message != "custom hint" {
		t.Errorf("diagnostics not carried over: %+v", data.Diagnostics)
	}
}

func TestBuildFallsBackToNow(t *testing.T) {
	data := Build(probe.Report{}, nil, &AgentInfo{})
	if data.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}
	if data.Summary.Status != "pass" {
		t.Errorf("empty report summary status = %q, want pass", data.Summary.Status)
	}
	if data.Instance != (Instance{}) || data.Network != (Network{}) {
		t.Error("empty report should produce empty instance and network")
	}
}

func TestAgentInfoFromRuntime(t *testing.T) {
	agent := AgentInfoFromRuntime()
	if agent.Name != "vpc-proof" {
		t.Errorf("Name = %q, want vpc-proof", agent.Name)
	}
	if agent.Version == "" || agent.GoVersion == "" || agent.Platform == "" {
		t.Errorf("runtime info incomplete: %+v", agent)
	}
}
