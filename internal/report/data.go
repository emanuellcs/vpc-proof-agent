package report

import (
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/diagnostic"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

// Data is the unified data model rendered by every formatter.
//
// It encapsulates everything needed for a complete evidence report: agent
// version, instance metadata, a network summary, probe results, diagnostic
// hints, the execution timestamp, and a high-level summary.
type Data struct {
	// Agent identifies the tool that produced the report.
	Agent AgentInfo `json:"agent"`
	// Instance holds the EC2 instance metadata.
	Instance Instance `json:"instance"`
	// Network holds the derived network summary.
	Network Network `json:"network"`
	// Summary aggregates the probe outcomes.
	Summary Summary `json:"summary"`
	// Probes is the full structured probe report.
	Probes probe.Report `json:"probes"`
	// Diagnostics are the troubleshooting hints produced by the engine.
	Diagnostics []diagnostic.Hint `json:"diagnostics"`
	// GeneratedAt is when the underlying probe run started.
	GeneratedAt time.Time `json:"generated_at"`
}

// AgentInfo identifies the version of the tool that generated the report.
type AgentInfo struct {
	// Name is the tool name.
	Name string `json:"name"`
	// Version is the semantic version.
	Version string `json:"version"`
	// Commit is the git commit the binary was built from.
	Commit string `json:"commit"`
	// BuildDate is the UTC build timestamp.
	BuildDate string `json:"build_date"`
	// GoVersion is the Go runtime version.
	GoVersion string `json:"go_version"`
	// Platform is GOOS/GOARCH.
	Platform string `json:"platform"`
}

// Instance holds the EC2 instance metadata gathered by the probes.
type Instance struct {
	// InstanceID is the EC2 instance identifier.
	InstanceID string `json:"instance_id,omitempty"`
	// AvailabilityZone is the placement availability zone.
	AvailabilityZone string `json:"availability_zone,omitempty"`
	// PrivateIP is the primary private IPv4 address.
	PrivateIP string `json:"private_ip,omitempty"`
	// PublicIP is the public IPv4 address.
	PublicIP string `json:"public_ip,omitempty"`
	// MACAddress is the primary network interface MAC address.
	MACAddress string `json:"mac_address,omitempty"`
	// SubnetCIDR is the expected subnet CIDR.
	SubnetCIDR string `json:"subnet_cidr,omitempty"`
	// VpcCIDR is the expected VPC CIDR.
	VpcCIDR string `json:"vpc_cidr,omitempty"`
}

// Network holds the derived network summary.
type Network struct {
	// DefaultGateway is the default route gateway, if any.
	DefaultGateway string `json:"default_gateway,omitempty"`
	// DefaultInterface is the interface carrying the default route.
	DefaultInterface string `json:"default_interface,omitempty"`
	// PrimaryIP is the primary interface IPv4 address.
	PrimaryIP string `json:"primary_ip,omitempty"`
	// DNSAddresses are the addresses the DNS probe resolved.
	DNSAddresses string `json:"dns_addresses,omitempty"`
}

// Summary aggregates probe outcomes.
type Summary struct {
	// Status is the overall report status.
	Status string `json:"status"`
	// Passed, Failed, Warned, Skipped are probe outcome counts.
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Warned  int `json:"warned"`
	Skipped int `json:"skipped"`
	// Total is the number of executed probes.
	Total int `json:"total"`
	// Duration is the total probe run duration.
	Duration time.Duration `json:"duration"`
}

// Build assembles a Data from a probe report, diagnostic hints, and
// agent metadata. Instance and network details are extracted from the
// individual probe results' technical details.
func Build(pr probe.Report, hints []diagnostic.Hint, agent *AgentInfo) Data {
	generatedAt := pr.StartedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}

	return Data{
		Agent:       *agent,
		Instance:    extractInstance(pr),
		Network:     extractNetwork(pr),
		Summary:     summarize(pr),
		Probes:      pr,
		Diagnostics: hints,
		GeneratedAt: generatedAt,
	}
}

// extractInstance pulls instance metadata out of the probe results.
func extractInstance(pr probe.Report) Instance {
	instance := Instance{}
	if result, ok := pr.Result(probe.MetadataProbeID); ok {
		instance.InstanceID = result.Details["instance_id"]
		instance.PrivateIP = result.Details["private_ip"]
		instance.PublicIP = result.Details["public_ip"]
		instance.AvailabilityZone = result.Details["availability_zone"]
		instance.MACAddress = result.Details["mac_address"]
	}
	if result, ok := pr.Result(probe.VPCOwnershipProbeID); ok {
		instance.VpcCIDR = result.Details["vpc_cidr"]
	}
	if result, ok := pr.Result(probe.SubnetOwnershipProbeID); ok {
		instance.SubnetCIDR = result.Details["subnet_cidr"]
	}
	return instance
}

// extractNetwork pulls the network summary out of the probe results.
func extractNetwork(pr probe.Report) Network {
	network := Network{}
	if result, ok := pr.Result(probe.DefaultRouteProbeID); ok {
		network.DefaultGateway = result.Details["gateway"]
		network.DefaultInterface = result.Details["interface"]
		network.PrimaryIP = result.Details["primary_ip"]
	}
	if result, ok := pr.Result(probe.DNSProbeID); ok {
		network.DNSAddresses = result.Details["addresses"]
	}
	return network
}

// summarize computes outcome counts and the overall status.
func summarize(pr probe.Report) Summary {
	summary := Summary{
		Status:   pr.Status.String(),
		Total:    len(pr.Results),
		Duration: pr.Duration,
	}
	for _, result := range pr.Results {
		switch result.Status {
		case probe.StatusPass:
			summary.Passed++
		case probe.StatusFail:
			summary.Failed++
		case probe.StatusWarn:
			summary.Warned++
		case probe.StatusSkip:
			summary.Skipped++
		}
	}
	return summary
}
