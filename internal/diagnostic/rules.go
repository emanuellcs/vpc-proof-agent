package diagnostic

import (
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

// defaultRules returns the built-in rule matrix.
//
// The engine is designed for extensibility: adding a new rule (or adjusting
// an existing one) is a matter of appending or editing an entry in this
// slice. The evaluation logic in Analyze is generic and never changes.
func defaultRules() []Rule {
	return []Rule{
		{
			// Outbound HTTPS fails while DNS works: the routing path to the
			// internet is broken, pointing at the IGW and route table.
			ID: "igw-route-table",
			Requires: map[string]probe.Status{
				probe.InternetHTTPSProbeID: probe.StatusFail,
				probe.DNSProbeID:           probe.StatusPass,
			},
			Severity: SeverityCritical,
			Hints: []string{
				"Check if the Internet Gateway is attached to the VPC and if the Route Table has a 0.0.0.0/0 route.",
			},
		},
		{
			ID:       "subnet-placement",
			Requires: map[string]probe.Status{probe.SubnetOwnershipProbeID: probe.StatusFail},
			Severity: SeverityCritical,
			Hints: []string{
				"Verify the EC2 instance was launched in the correct Subnet (10.0.1.0/24).",
			},
		},
		{
			ID:       "public-ip-assignment",
			Requires: map[string]probe.Status{probe.PublicIPConsistencyProbeID: probe.StatusFail},
			Severity: SeverityWarning,
			Hints: []string{
				"Ensure the Subnet has 'Auto-assign public IP' enabled and the instance has a public IP associated.",
			},
		},
		{
			ID:       "default-route",
			Requires: map[string]probe.Status{probe.DefaultRouteProbeID: probe.StatusFail},
			Severity: SeverityCritical,
			Hints: []string{
				"Verify the Route Table has a 0.0.0.0/0 route to the Internet Gateway and is associated with the subnet.",
			},
		},
		{
			ID:       "vpc-mismatch",
			Requires: map[string]probe.Status{probe.VPCOwnershipProbeID: probe.StatusFail},
			Severity: SeverityCritical,
			Hints: []string{
				"Verify the instance's VPC matches the expected VPC (10.0.0.0/16).",
			},
		},
		{
			ID:       "dns-configuration",
			Requires: map[string]probe.Status{probe.DNSProbeID: probe.StatusFail},
			Severity: SeverityWarning,
			Hints: []string{
				"Check the VPC DNS settings and DHCP option sets.",
			},
		},
		{
			ID:       "imds-configuration",
			Requires: map[string]probe.Status{probe.MetadataProbeID: probe.StatusFail},
			Severity: SeverityWarning,
			Hints: []string{
				"Ensure IMDSv2 is enabled on the instance and reachable from within the instance.",
			},
		},
		{
			ID:       "clock-skew-critical",
			Requires: map[string]probe.Status{probe.ClockSkewProbeID: probe.StatusFail},
			Severity: SeverityCritical,
			Hints: []string{
				"Clock skew detected. Verify NTP configuration and ensure the instance is synchronized with Amazon Time Sync Service.",
			},
		},
		{
			ID:       "clock-skew-warning",
			Requires: map[string]probe.Status{probe.ClockSkewProbeID: probe.StatusWarn},
			Severity: SeverityWarning,
			Hints: []string{
				"Clock skew detected. Verify NTP configuration and ensure the instance is synchronized with Amazon Time Sync Service.",
			},
		},
		{
			ID:       "system-resources",
			Requires: map[string]probe.Status{probe.SystemResourcesProbeID: probe.StatusWarn},
			Severity: SeverityWarning,
			Hints: []string{
				"Check available memory and system load on the instance. Free up resources or resize the instance if needed.",
			},
		},
	}
}
