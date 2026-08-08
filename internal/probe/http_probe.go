package probe

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/pkg/metadata"
	"github.com/emanuellcs/vpc-proof-agent/pkg/netutil"
)

// internetHTTPSHint is attached when outbound HTTPS connectivity fails.
const internetHTTPSHint = "Check if the Internet Gateway is attached to the VPC and if the Route Table has a 0.0.0.0/0 route."

// publicIPHint is attached when the AWS-reported and externally-observed
// public IPs disagree.
const publicIPHint = "Ensure the Subnet has 'Auto-assign public IP' enabled and the instance has a public IP associated."

// InternetHTTPSProbe verifies outbound internet connectivity by fetching the
// configured external echo service over HTTPS.
type InternetHTTPSProbe struct {
	client     *http.Client
	url        string
	maxRetries int
	timeout    time.Duration
	logger     *observability.Logger
}

// NewInternetHTTPSProbe builds an InternetHTTPSProbe.
func NewInternetHTTPSProbe(client *http.Client, url string, maxRetries int, timeout time.Duration, logger *observability.Logger) *InternetHTTPSProbe {
	return &InternetHTTPSProbe{client: client, url: url, maxRetries: maxRetries, timeout: timeout, logger: logger}
}

// ID returns the probe identifier.
func (p *InternetHTTPSProbe) ID() string {
	return InternetHTTPSProbeID
}

// Execute fetches the echo service and reports connectivity.
func (p *InternetHTTPSProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	result := Result{
		ID:      p.ID(),
		Name:    "Outbound HTTPS",
		Details: map[string]string{},
	}

	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	body, status, err := netutil.HTTPGet(ctx, p.client, p.url, p.maxRetries)
	if err != nil {
		result.Status = StatusFail
		result.Duration = time.Since(start)
		result.Message = fmt.Sprintf("could not reach %s: %v", p.url, err)
		result.Hint = internetHTTPSHint
		result.Details["url"] = p.url
		return result
	}

	result.Status = StatusPass
	result.Duration = time.Since(start)
	result.Message = fmt.Sprintf("outbound HTTPS to %s succeeded", p.url)
	result.Details["url"] = p.url
	result.Details["status"] = strconv.Itoa(status)
	result.Details["external_ip"] = body

	if p.logger != nil {
		p.logger.Debug("outbound https verified",
			observability.Component("probe"),
			observability.Str("url", p.url),
			observability.Int("status", status),
		)
	}
	return result
}

// PublicIPConsistencyProbe compares the public IP reported by IMDS with the
// public IP observed by an external echo service.
type PublicIPConsistencyProbe struct {
	client     metadata.Client
	httpClient *http.Client
	url        string
	timeout    time.Duration
	logger     *observability.Logger
}

// NewPublicIPConsistencyProbe builds a PublicIPConsistencyProbe.
func NewPublicIPConsistencyProbe(client metadata.Client, httpClient *http.Client, url string, timeout time.Duration, logger *observability.Logger) *PublicIPConsistencyProbe {
	return &PublicIPConsistencyProbe{client: client, httpClient: httpClient, url: url, timeout: timeout, logger: logger}
}

// ID returns the probe identifier.
func (p *PublicIPConsistencyProbe) ID() string {
	return PublicIPConsistencyProbeID
}

// Execute compares the metadata public IP with the externally observed IP.
func (p *PublicIPConsistencyProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	result := Result{
		ID:      p.ID(),
		Name:    "Public IP consistency",
		Details: map[string]string{},
	}

	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	metaIP, err := p.client.PublicIP(ctx)
	if err != nil {
		return p.failed(&result, start, "failed to fetch public IP from metadata", err)
	}

	externalIP, status, err := netutil.HTTPGet(ctx, p.httpClient, p.url, 0)
	if err != nil {
		return p.failed(&result, start, "failed to reach the external echo service", err)
	}

	result.Details["metadata_public_ip"] = metaIP
	result.Details["external_public_ip"] = externalIP
	result.Details["echo_status"] = strconv.Itoa(status)

	switch {
	case metaIP == "":
		result.Status = StatusWarn
		result.Duration = time.Since(start)
		result.Message = "instance has no public IP assigned"
		result.Hint = publicIPHint
	case externalIP == "":
		result.Status = StatusWarn
		result.Duration = time.Since(start)
		result.Message = "echo service returned an empty public IP"
	case metaIP == externalIP:
		result.Status = StatusPass
		result.Duration = time.Since(start)
		result.Message = fmt.Sprintf("metadata public IP %s matches the external view", metaIP)
	default:
		result.Status = StatusFail
		result.Duration = time.Since(start)
		result.Message = fmt.Sprintf("metadata public IP %s differs from the external view %s", metaIP, externalIP)
		result.Hint = publicIPHint
	}

	if p.logger != nil {
		p.logger.Debug("public ip consistency checked",
			observability.Component("probe"),
			observability.Str("metadata_ip", metaIP),
			observability.Str("external_ip", externalIP),
		)
	}
	return result
}

func (p *PublicIPConsistencyProbe) failed(result *Result, start time.Time, message string, err error) Result {
	result.Status = StatusFail
	result.Duration = time.Since(start)
	result.Message = fmt.Sprintf("%s: %v", message, err)
	result.Hint = publicIPHint
	if p.logger != nil {
		p.logger.Debug(message, observability.Component("probe"), observability.Error(err))
	}
	return *result
}
