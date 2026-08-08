package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/pkg/metadata"
)

// metadataHint is attached when metadata cannot be fetched.
const metadataHint = "Ensure IMDSv2 is enabled on the instance and reachable from within the instance."

// MetadataProbe validates IMDSv2 accessibility and fetches the instance's
// base identity.
type MetadataProbe struct {
	client metadata.Client
	logger *observability.Logger
}

// NewMetadataProbe builds a MetadataProbe.
func NewMetadataProbe(client metadata.Client, logger *observability.Logger) *MetadataProbe {
	return &MetadataProbe{client: client, logger: logger}
}

// ID returns the probe identifier.
func (p *MetadataProbe) ID() string {
	return MetadataProbeID
}

// Execute fetches the base identity fields and reports accessibility.
func (p *MetadataProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	result := Result{
		ID:      p.ID(),
		Name:    "IMDSv2 metadata",
		Details: map[string]string{},
	}

	instanceID, err := p.client.InstanceID(ctx)
	if err != nil {
		return p.failed(&result, start, "failed to fetch instance ID", err)
	}
	privateIP, err := p.client.PrivateIP(ctx)
	if err != nil {
		return p.failed(&result, start, "failed to fetch private IP", err)
	}
	publicIP, err := p.client.PublicIP(ctx)
	if err != nil {
		return p.failed(&result, start, "failed to fetch public IP", err)
	}
	az, err := p.client.AvailabilityZone(ctx)
	if err != nil {
		return p.failed(&result, start, "failed to fetch availability zone", err)
	}

	result.Status = StatusPass
	result.Duration = time.Since(start)
	result.Message = "IMDSv2 metadata accessible"
	result.Details["instance_id"] = instanceID
	result.Details["private_ip"] = privateIP
	result.Details["public_ip"] = publicIP
	result.Details["availability_zone"] = az

	if p.logger != nil {
		p.logger.Debug("metadata probe completed",
			observability.Component("probe"),
			observability.Str("instance_id", instanceID),
		)
	}
	return result
}

func (p *MetadataProbe) failed(result *Result, start time.Time, message string, err error) Result {
	result.Status = StatusFail
	result.Duration = time.Since(start)
	result.Message = fmt.Sprintf("%s: %v", message, err)
	result.Hint = metadataHint
	if p.logger != nil {
		p.logger.Debug("metadata probe failed", observability.Component("probe"), observability.Error(err))
	}
	return *result
}
