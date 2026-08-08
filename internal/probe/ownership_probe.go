package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/pkg/cidr"
	"github.com/emanuellcs/vpc-proof-agent/pkg/metadata"
)

// vpcOwnershipHint is attached when the private IP is outside the VPC CIDR.
const vpcOwnershipHint = "Verify the instance's VPC matches the expected VPC (10.0.0.0/16)."

// subnetOwnershipHint is attached when the private IP is outside the subnet
// CIDR.
const subnetOwnershipHint = "Verify the EC2 instance was launched in the correct Subnet (10.0.1.0/24)."

// VPCOwnershipProbe verifies that the instance's private IP belongs to the
// expected VPC CIDR.
type VPCOwnershipProbe struct {
	client  metadata.Client
	vpcCIDR string
	logger  *observability.Logger
}

// NewVPCOwnershipProbe builds a VPCOwnershipProbe.
func NewVPCOwnershipProbe(client metadata.Client, vpcCIDR string, logger *observability.Logger) *VPCOwnershipProbe {
	return &VPCOwnershipProbe{client: client, vpcCIDR: vpcCIDR, logger: logger}
}

// ID returns the probe identifier.
func (p *VPCOwnershipProbe) ID() string {
	return VPCOwnershipProbeID
}

// Execute checks that the private IP is inside the expected VPC CIDR.
func (p *VPCOwnershipProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	result := Result{
		ID:      p.ID(),
		Name:    "VPC ownership",
		Details: map[string]string{},
	}

	privateIP, err := p.client.PrivateIP(ctx)
	if err != nil {
		p.logFailure("failed to fetch private IP", err)
		return ownershipFailed(&result, start, "failed to fetch private IP", err)
	}

	contained, err := cidr.Contains(p.vpcCIDR, privateIP)
	if err != nil {
		p.logFailure("invalid expected VPC CIDR", err)
		return ownershipFailed(&result, start, "invalid expected VPC CIDR", err)
	}
	if !contained {
		result.Status = StatusFail
		result.Duration = time.Since(start)
		result.Message = fmt.Sprintf("private IP %s is not inside VPC %s", privateIP, p.vpcCIDR)
		result.Hint = vpcOwnershipHint
		result.Details["private_ip"] = privateIP
		result.Details["vpc_cidr"] = p.vpcCIDR
		return result
	}

	result.Status = StatusPass
	result.Duration = time.Since(start)
	result.Message = fmt.Sprintf("private IP %s is inside VPC %s", privateIP, p.vpcCIDR)
	result.Details["private_ip"] = privateIP
	result.Details["vpc_cidr"] = p.vpcCIDR
	p.logDebug("vpc ownership verified", result.Message)
	return result
}

// logDebug records a debug event when a logger is configured.
func (p *VPCOwnershipProbe) logDebug(msg, detail string) {
	if p.logger != nil {
		p.logger.Debug(msg, observability.Component("probe"), observability.Str("detail", detail))
	}
}

// logFailure records a failure event when a logger is configured.
func (p *VPCOwnershipProbe) logFailure(msg string, err error) {
	if p.logger != nil {
		p.logger.Debug(msg, observability.Component("probe"), observability.Error(err))
	}
}

// SubnetOwnershipProbe verifies that the instance's private IP belongs to the
// expected subnet CIDR.
type SubnetOwnershipProbe struct {
	client     metadata.Client
	subnetCIDR string
	logger     *observability.Logger
}

// NewSubnetOwnershipProbe builds a SubnetOwnershipProbe.
func NewSubnetOwnershipProbe(client metadata.Client, subnetCIDR string, logger *observability.Logger) *SubnetOwnershipProbe {
	return &SubnetOwnershipProbe{client: client, subnetCIDR: subnetCIDR, logger: logger}
}

// ID returns the probe identifier.
func (p *SubnetOwnershipProbe) ID() string {
	return SubnetOwnershipProbeID
}

// Execute checks that the private IP is inside the expected subnet CIDR.
func (p *SubnetOwnershipProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	result := Result{
		ID:      p.ID(),
		Name:    "Subnet ownership",
		Details: map[string]string{},
	}

	privateIP, err := p.client.PrivateIP(ctx)
	if err != nil {
		return ownershipFailed(&result, start, "failed to fetch private IP", err)
	}

	contained, err := cidr.Contains(p.subnetCIDR, privateIP)
	if err != nil {
		return ownershipFailed(&result, start, "invalid expected subnet CIDR", err)
	}
	if !contained {
		result.Status = StatusFail
		result.Duration = time.Since(start)
		result.Message = fmt.Sprintf("private IP %s is not inside subnet %s", privateIP, p.subnetCIDR)
		result.Hint = subnetOwnershipHint
		result.Details["private_ip"] = privateIP
		result.Details["subnet_cidr"] = p.subnetCIDR
		return result
	}

	result.Status = StatusPass
	result.Duration = time.Since(start)
	result.Message = fmt.Sprintf("private IP %s is inside subnet %s", privateIP, p.subnetCIDR)
	result.Details["private_ip"] = privateIP
	result.Details["subnet_cidr"] = p.subnetCIDR
	p.logDebug("subnet ownership verified", result.Message)
	return result
}

// logDebug records a debug event when a logger is configured.
func (p *SubnetOwnershipProbe) logDebug(msg, detail string) {
	if p.logger != nil {
		p.logger.Debug(msg, observability.Component("probe"), observability.Str("detail", detail))
	}
}

func ownershipFailed(result *Result, start time.Time, message string, err error) Result {
	result.Status = StatusFail
	result.Duration = time.Since(start)
	result.Message = fmt.Sprintf("%s: %v", message, err)
	result.Hint = metadataHint
	return *result
}
