package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// imdsMock simulates the EC2 instance metadata service, enforcing the
// IMDSv2 token handshake.
type imdsMock struct {
	token    string
	values   map[string]string
	notFound map[string]bool
	status   map[string]int
	sleepGet time.Duration

	// unauthorizedFirst makes the first GET return 401 (forcing a token
	// refresh and retry).
	unauthorizedFirst bool
	// emptyToken makes the token PUT respond with an empty body.
	emptyToken bool
	// putFailsAfter makes every token PUT after the given number of
	// successful PUTs fail with a 500.
	putFailsAfter int

	putCount atomic.Int32
	getCount atomic.Int32
	firstGet atomic.Bool
}

func newIMDSMock(token string) *imdsMock {
	return &imdsMock{
		token:    token,
		values:   map[string]string{},
		notFound: map[string]bool{},
		status:   map[string]int{},
		firstGet: atomic.Bool{},
	}
}

func (m *imdsMock) handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPut && r.URL.Path == tokenPath:
		if r.Header.Get(ttlHeader) == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		m.putCount.Add(1)
		if m.putFailsAfter > 0 && int(m.putCount.Load()) > m.putFailsAfter {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if m.emptyToken {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(m.token))
	case r.Method == http.MethodGet:
		m.getCount.Add(1)
		if r.Header.Get(tokenHeader) != m.token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if m.unauthorizedFirst && m.firstGet.CompareAndSwap(false, true) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if m.sleepGet > 0 {
			time.Sleep(m.sleepGet)
		}
		if m.notFound[r.URL.Path] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if status, ok := m.status[r.URL.Path]; ok {
			w.WriteHeader(status)
			return
		}
		if body, ok := m.values[r.URL.Path]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestServer(t *testing.T, mock *imdsMock) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(mock.handler))
	t.Cleanup(server.Close)
	return server
}

func newTestClient(t *testing.T, mock *imdsMock) (*client, *httptest.Server) {
	t.Helper()
	server := newTestServer(t, mock)
	return New(Options{BaseURL: server.URL}).(*client), server
}

const testToken = "AQEA-test-session-token"

func TestClientFetchesAllDataPoints(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.values = map[string]string{
		"/latest/meta-data/instance-id":                 "i-0123456789abcdef0",
		"/latest/meta-data/local-ipv4":                  "10.0.1.42",
		"/latest/meta-data/public-ipv4":                 "203.0.113.7",
		"/latest/meta-data/placement/availability-zone": "us-east-1a",
		"/latest/meta-data/mac":                         "0a:1b:2c:3d:4e:5f",
		"/latest/meta-data/network/interfaces/macs/0a:1b:2c:3d:4e:5f/subnet-ipv4-cidr-block": "10.0.1.0/24",
		"/latest/meta-data/network/interfaces/macs/0a:1b:2c:3d:4e:5f/vpc-ipv4-cidr-block":    "10.0.0.0/16",
	}
	c, _ := newTestClient(t, mock)
	ctx := context.Background()

	tests := []struct {
		name string
		got  func() (string, error)
		want string
	}{
		{"instance id", func() (string, error) { return c.InstanceID(ctx) }, "i-0123456789abcdef0"},
		{"private ip", func() (string, error) { return c.PrivateIP(ctx) }, "10.0.1.42"},
		{"public ip", func() (string, error) { return c.PublicIP(ctx) }, "203.0.113.7"},
		{"availability zone", func() (string, error) { return c.AvailabilityZone(ctx) }, "us-east-1a"},
		{"mac", func() (string, error) { return c.MACAddress(ctx) }, "0a:1b:2c:3d:4e:5f"},
		{"subnet cidr", func() (string, error) { return c.SubnetCIDR(ctx) }, "10.0.1.0/24"},
		{"vpc cidr", func() (string, error) { return c.VpcCIDR(ctx) }, "10.0.0.0/16"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.got()
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
			}
		})
	}

	if got := mock.putCount.Load(); got != 1 {
		t.Errorf("expected a single token PUT (cached), got %d", got)
	}
}

func TestClientTokenReuseAcrossCalls(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.values["/latest/meta-data/instance-id"] = "i-00000000000000000"
	c, _ := newTestClient(t, mock)
	ctx := context.Background()

	for i := range 5 {
		if _, err := c.InstanceID(ctx); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	if got := mock.putCount.Load(); got != 1 {
		t.Errorf("expected 1 token PUT across 5 calls, got %d", got)
	}
	if got := mock.getCount.Load(); got != 5 {
		t.Errorf("expected 5 GETs, got %d", got)
	}
}

func TestClientRefreshesExpiredToken(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.values["/latest/meta-data/instance-id"] = "i-11111111111111111"
	c, _ := newTestClient(t, mock)
	ctx := context.Background()

	if _, err := c.InstanceID(ctx); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Force the cached token to be treated as expired.
	c.tokenMu.Lock()
	c.tokenExpiry = time.Now().Add(-time.Second)
	c.tokenMu.Unlock()

	if _, err := c.InstanceID(ctx); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if got := mock.putCount.Load(); got != 2 {
		t.Errorf("expected a token re-PUT after expiry, got %d", got)
	}
}

func TestClientRefreshesTokenOnUnauthorized(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.values["/latest/meta-data/instance-id"] = "i-22222222222222222"
	mock.unauthorizedFirst = true
	c, _ := newTestClient(t, mock)

	got, err := c.InstanceID(context.Background())
	if err != nil {
		t.Fatalf("expected transparent retry after 401, got %v", err)
	}
	if got != "i-22222222222222222" {
		t.Errorf("instance id = %q, want i-22222222222222222", got)
	}
	if mock.putCount.Load() != 2 {
		t.Errorf("expected token re-PUT after 401, got %d", mock.putCount.Load())
	}
}

func TestPublicIPMissingReturnsEmpty(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.notFound["/latest/meta-data/public-ipv4"] = true
	c, _ := newTestClient(t, mock)

	got, err := c.PublicIP(context.Background())
	if err != nil {
		t.Fatalf("missing public IP should not error, got %v", err)
	}
	if got != "" {
		t.Errorf("public ip = %q, want empty", got)
	}
}

func TestGetReturnsErrorOnNotFound(t *testing.T) {
	mock := newIMDSMock(testToken)
	c, _ := newTestClient(t, mock)

	_, err := c.InstanceID(context.Background())
	if err == nil {
		t.Fatal("expected error for missing metadata path, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status 404, got %v", err)
	}
}

func TestTokenRequestFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	c := New(Options{BaseURL: server.URL})
	if _, err := c.InstanceID(context.Background()); err == nil {
		t.Fatal("expected error when the token request fails, got nil")
	}
}

func TestContextCancellation(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.values["/latest/meta-data/instance-id"] = "i-33333333333333333"
	mock.sleepGet = 500 * time.Millisecond
	c, _ := newTestClient(t, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.InstanceID(ctx)
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("error should mention the deadline, got %v", err)
	}
}

func TestDefaultOptions(t *testing.T) {
	c := New(Options{}).(*client)
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, defaultBaseURL)
	}
	if c.ttlSeconds != defaultTokenTTL {
		t.Errorf("ttlSeconds = %d, want %d", c.ttlSeconds, defaultTokenTTL)
	}
	if c.httpClient.Timeout != defaultHTTPTimeout {
		t.Errorf("httpClient.Timeout = %s, want %s", c.httpClient.Timeout, defaultHTTPTimeout)
	}
}

func TestCustomOptions(t *testing.T) {
	httpClient := &http.Client{Timeout: 42 * time.Second}
	c := New(Options{BaseURL: "http://127.0.0.1:4566", TokenTTLSeconds: 60, HTTPClient: httpClient}).(*client)
	if c.baseURL != "http://127.0.0.1:4566" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.ttlSeconds != 60 {
		t.Errorf("ttlSeconds = %d, want 60", c.ttlSeconds)
	}
	if c.httpClient != httpClient {
		t.Error("custom http client not used")
	}
}

func TestBaseURLTrailingSlashTrimmed(t *testing.T) {
	c := New(Options{BaseURL: "http://127.0.0.1:4566/"}).(*client)
	if c.baseURL != "http://127.0.0.1:4566" {
		t.Errorf("baseURL = %q, trailing slash not trimmed", c.baseURL)
	}
}

func TestPublicIPRejectsNon200Non404(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.status["/latest/meta-data/public-ipv4"] = http.StatusForbidden
	c, _ := newTestClient(t, mock)

	_, err := c.PublicIP(context.Background())
	if err == nil {
		t.Fatal("expected error for 403 public-ipv4, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention status 403, got %v", err)
	}
}

func TestSubnetCIDRMissingMAC(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.notFound["/latest/meta-data/mac"] = true
	c, _ := newTestClient(t, mock)

	if _, err := c.SubnetCIDR(context.Background()); err == nil {
		t.Fatal("expected error when the MAC address is missing, got nil")
	}
}

func TestVpcCIDRMissingMAC(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.notFound["/latest/meta-data/mac"] = true
	c, _ := newTestClient(t, mock)

	if _, err := c.VpcCIDR(context.Background()); err == nil {
		t.Fatal("expected error when the MAC address is missing, got nil")
	}
}

func TestEmptySessionToken(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.emptyToken = true
	c, _ := newTestClient(t, mock)

	_, err := c.InstanceID(context.Background())
	if err == nil {
		t.Fatal("expected error for an empty session token, got nil")
	}
	if !strings.Contains(err.Error(), "empty session token") {
		t.Errorf("error should mention the empty token, got %v", err)
	}
}

func TestFetchTokenBuildError(t *testing.T) {
	c := New(Options{BaseURL: "://invalid"})
	if _, err := c.InstanceID(context.Background()); err == nil {
		t.Fatal("expected error for an invalid base URL, got nil")
	}
}

func TestFetchTokenTransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := server.URL
	server.Close()

	c := New(Options{BaseURL: url})
	if _, err := c.InstanceID(context.Background()); err == nil {
		t.Fatal("expected error when the metadata service is unreachable, got nil")
	}
}

type errorRoundTripper struct{}

func (errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("transport failure")
}

func TestGetTransportErrorAfterTokenCached(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.values["/latest/meta-data/instance-id"] = "i-44444444444444444"
	c, _ := newTestClient(t, mock)

	if _, err := c.InstanceID(context.Background()); err != nil {
		t.Fatalf("cache token: %v", err)
	}

	c.httpClient = &http.Client{Transport: errorRoundTripper{}}
	if _, err := c.InstanceID(context.Background()); err == nil {
		t.Fatal("expected transport error for the metadata GET, got nil")
	}
}

func TestGetBuildErrorAfterTokenCached(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.values["/latest/meta-data/instance-id"] = "i-55555555555555555"
	c, _ := newTestClient(t, mock)

	if _, err := c.InstanceID(context.Background()); err != nil {
		t.Fatalf("cache token: %v", err)
	}

	c.baseURL = "://invalid"
	if _, err := c.InstanceID(context.Background()); err == nil {
		t.Fatal("expected build error for an invalid base URL, got nil")
	}
}

func TestUnauthorizedThenTokenFetchFails(t *testing.T) {
	mock := newIMDSMock(testToken)
	mock.values["/latest/meta-data/instance-id"] = "i-66666666666666666"
	mock.unauthorizedFirst = true
	mock.putFailsAfter = 1
	c, _ := newTestClient(t, mock)

	if _, err := c.InstanceID(context.Background()); err == nil {
		t.Fatal("expected error when the token refresh fails after a 401, got nil")
	}
}
