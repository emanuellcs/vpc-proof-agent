package metadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// defaultBaseURL is the EC2 instance metadata service link-local address.
	defaultBaseURL = "http://169.254.169.254"
	// defaultTokenTTL is the default requested IMDSv2 session token lifetime.
	defaultTokenTTL = 21600
	// defaultHTTPTimeout bounds every HTTP request when no client is supplied.
	defaultHTTPTimeout = 2 * time.Second

	tokenPath   = "/latest/api/token" // #nosec G101 -- URL path, not a credential
	ttlHeader   = "X-aws-ec2-metadata-token-ttl-seconds"
	tokenHeader = "X-aws-ec2-metadata-token" // #nosec G101 -- header name, not a credential
)

// Options configures the metadata client.
type Options struct {
	// BaseURL overrides the IMDS endpoint. Defaults to the EC2 link-local
	// address (http://169.254.169.254); tests point it at an httptest server.
	BaseURL string
	// TokenTTLSeconds is the requested IMDSv2 session token lifetime in
	// seconds. Defaults to 21600.
	TokenTTLSeconds int
	// HTTPClient performs all requests. When nil, a client with a 2s timeout
	// is used.
	HTTPClient *http.Client
}

// client implements the IMDSv2 protocol.
//
// The protocol is strict: a session token is obtained via a PUT to
// /latest/api/token carrying the TTL header, and every subsequent GET carries
// that token in the X-aws-ec2-metadata-token header. The token is cached
// until expiry and re-negotiated lazily. The token value is never included in
// error messages or exposed to loggers.
type client struct {
	baseURL    string
	ttlSeconds int
	httpClient *http.Client

	tokenMu     sync.Mutex
	token       string
	tokenExpiry time.Time
}

// New builds a Client from opts, applying defaults for unset fields.
func New(opts Options) Client {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	ttl := opts.TokenTTLSeconds
	if ttl <= 0 {
		ttl = defaultTokenTTL
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	return &client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		ttlSeconds: ttl,
		httpClient: httpClient,
	}
}

// Get fetches the metadata value at path (relative to the metadata root).
// A non-200 response is returned as an error.
func (c *client) Get(ctx context.Context, path string) (string, error) {
	body, status, err := c.get(ctx, path)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("metadata request %s failed with status %d", path, status)
	}
	return body, nil
}

// InstanceID returns the EC2 instance ID.
func (c *client) InstanceID(ctx context.Context) (string, error) {
	return c.Get(ctx, "/latest/meta-data/instance-id")
}

// PrivateIP returns the primary private IPv4 address.
func (c *client) PrivateIP(ctx context.Context) (string, error) {
	return c.Get(ctx, "/latest/meta-data/local-ipv4")
}

// AvailabilityZone returns the placement availability zone.
func (c *client) AvailabilityZone(ctx context.Context) (string, error) {
	return c.Get(ctx, "/latest/meta-data/placement/availability-zone")
}

// MACAddress returns the primary network interface MAC address.
func (c *client) MACAddress(ctx context.Context) (string, error) {
	return c.Get(ctx, "/latest/meta-data/mac")
}

// PublicIP returns the public IPv4 address, or an empty string when the
// instance has no public IP (the metadata service responds with 404).
func (c *client) PublicIP(ctx context.Context) (string, error) {
	body, status, err := c.get(ctx, "/latest/meta-data/public-ipv4")
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", nil
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("metadata request %s failed with status %d", "/latest/meta-data/public-ipv4", status)
	}
	return body, nil
}

// SubnetCIDR returns the CIDR block of the primary subnet, resolved through
// the primary interface MAC address.
func (c *client) SubnetCIDR(ctx context.Context) (string, error) {
	mac, err := c.MACAddress(ctx)
	if err != nil {
		return "", err
	}
	return c.Get(ctx, "/latest/meta-data/network/interfaces/macs/"+mac+"/subnet-ipv4-cidr-block")
}

// VpcCIDR returns the CIDR block of the VPC, resolved through the primary
// interface MAC address.
func (c *client) VpcCIDR(ctx context.Context) (string, error) {
	mac, err := c.MACAddress(ctx)
	if err != nil {
		return "", err
	}
	return c.Get(ctx, "/latest/meta-data/network/interfaces/macs/"+mac+"/vpc-ipv4-cidr-block")
}

// get performs a token-authenticated GET, transparently re-negotiating the
// session token when it is expired or rejected with 401.
func (c *client) get(ctx context.Context, path string) (body string, status int, err error) {
	token, err := c.sessionToken(ctx)
	if err != nil {
		return "", 0, err
	}

	body, status, err = c.getWithToken(ctx, path, token)
	if err != nil {
		return "", 0, err
	}
	if status == http.StatusUnauthorized {
		c.invalidateToken()
		refreshed, err := c.sessionToken(ctx)
		if err != nil {
			return "", 0, err
		}
		return c.getWithToken(ctx, path, refreshed)
	}
	return body, status, nil
}

// getWithToken issues a single token-authenticated GET and returns the raw
// body and status code.
func (c *client) getWithToken(ctx context.Context, path, token string) (body string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, http.NoBody)
	if err != nil {
		return "", 0, fmt.Errorf("metadata: build request: %w", err)
	}
	req.Header.Set(tokenHeader, token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("metadata: request %s: %w", path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("metadata: read response %s: %w", path, err)
	}
	return strings.TrimSpace(string(raw)), resp.StatusCode, nil
}

// sessionToken returns a valid session token, fetching a fresh one when the
// cache is empty or expired.
func (c *client) sessionToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token, nil
	}
	return c.fetchTokenLocked(ctx)
}

// fetchTokenLocked PUTs a new session token and caches it. The caller must
// hold tokenMu.
func (c *client) fetchTokenLocked(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+tokenPath, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("metadata: build token request: %w", err)
	}
	req.Header.Set(ttlHeader, strconv.Itoa(c.ttlSeconds))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata: token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata: token request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("metadata: read token response: %w", err)
	}
	token := strings.TrimSpace(string(body))
	if token == "" {
		return "", fmt.Errorf("metadata: empty session token")
	}

	c.token = token
	c.tokenExpiry = time.Now().Add(time.Duration(c.ttlSeconds) * time.Second)
	return token, nil
}

// invalidateToken drops the cached session token so the next request fetches
// a fresh one.
func (c *client) invalidateToken() {
	c.tokenMu.Lock()
	c.token = ""
	c.tokenExpiry = time.Time{}
	c.tokenMu.Unlock()
}
