//go:build e2e

// Package e2e exercises the compiled vpc-proof binary end to end: the CLI
// commands against local mock servers (IMDS and echo), and the real HTTP
// server lifecycle with authentication and graceful shutdown.
//
// Run with: go test -tags e2e -count=1 -v ./test/e2e/
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// binaryPath is the compiled vpc-proof binary, built once in TestMain.
var binaryPath string

func TestMain(m *testing.M) {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: find repo root: %v\n", err)
		os.Exit(1)
	}

	dir, err := os.MkdirTemp("", "vpc-proof-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: mkdir temp: %v\n", err)
		os.Exit(1)
	}
	binaryPath = filepath.Join(dir, "vpc-proof")

	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/vpc-proof")
	build.Dir = root
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build binary: %v\n", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(dir)
	os.Exit(code)
}

// repoRoot resolves the module root from the test package directory
// (<root>/test/e2e).
func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(wd, "..", ".."))
}

// mockIMDS implements the IMDSv2 token handshake and metadata paths. An empty
// publicIP makes the public-ipv4 path return 404 (no public IP).
func mockIMDS(t *testing.T, publicIP string) *httptest.Server {
	t.Helper()
	const token = "AQEA-e2e-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token":
			_, _ = w.Write([]byte(token))
		case r.Method == http.MethodGet && r.Header.Get("X-aws-ec2-metadata-token") == token:
			switch r.URL.Path {
			case "/latest/meta-data/instance-id":
				_, _ = w.Write([]byte("i-0e2e0000000000001"))
			case "/latest/meta-data/local-ipv4":
				_, _ = w.Write([]byte("10.0.1.42"))
			case "/latest/meta-data/public-ipv4":
				if publicIP == "" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = w.Write([]byte(publicIP))
			case "/latest/meta-data/placement/availability-zone":
				_, _ = w.Write([]byte("us-east-1a"))
			case "/latest/meta-data/mac":
				_, _ = w.Write([]byte("0a:1b:2c:3d:4e:5f"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// mockEcho returns a server that echoes the public IP body.
func mockEcho(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func waitForPort(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("port never became reachable")
}

// runCLI runs the compiled binary and returns stdout, stderr, and the exit
// code.
func runCLI(t *testing.T, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), envSlice(env)...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return stdout.String(), stderr.String(), code
}

func envSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}

func copyEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}

// hostHealthyForPass reports whether the host's OS-level probes pass, which is
// required for a clean exit-0 assertion in the E2E environment.
func hostHealthyForPass(t *testing.T, env map[string]string) bool {
	t.Helper()
	stdout, _, code := runCLI(t, env, "report", "--format", "json", "--output", "-")
	if code != 0 {
		return false
	}
	var doc struct {
		Probes struct {
			Results []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"results"`
		} `json:"probes"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		return false
	}
	status := map[string]string{}
	for _, result := range doc.Probes.Results {
		status[result.ID] = result.Status
	}
	return status["default_route"] == "pass" && status["system_resources"] == "pass"
}

func TestCLICheckExitCodes(t *testing.T) {
	imds := mockIMDS(t, "203.0.113.7")
	echo := mockEcho(t, "203.0.113.7")

	baseEnv := map[string]string{
		"VPC_PROOF_PROBES_IMDS_BASE_URL": imds.URL,
		"VPC_PROOF_PROBES_ECHO_URLS":     echo.URL,
		"VPC_PROOF_LOG_FORMAT":           "console",
	}

	t.Run("healthy exits zero", func(t *testing.T) {
		if !hostHealthyForPass(t, baseEnv) {
			t.Skip("host OS probes are not healthy enough for a full-pass assertion")
		}
		_, _, code := runCLI(t, baseEnv, "check")
		if code != 0 {
			t.Fatalf("check exit code = %d, want 0", code)
		}
	})

	t.Run("failure exits one", func(t *testing.T) {
		env := copyEnv(baseEnv)
		env["VPC_PROOF_PROBES_VPC_CIDR"] = "192.168.0.0/16"
		_, _, code := runCLI(t, env, "check")
		if code != 1 {
			t.Fatalf("check exit code = %d, want 1", code)
		}
	})

	t.Run("warning exits two", func(t *testing.T) {
		env := copyEnv(baseEnv)
		env["VPC_PROOF_PROBES_IMDS_BASE_URL"] = mockIMDS(t, "").URL
		_, _, code := runCLI(t, env, "check")
		if code != 2 {
			t.Fatalf("check exit code = %d, want 2", code)
		}
	})
}

func TestCLIReportEvidence(t *testing.T) {
	imds := mockIMDS(t, "203.0.113.7")
	echo := mockEcho(t, "203.0.113.7")

	env := map[string]string{
		"VPC_PROOF_PROBES_IMDS_BASE_URL": imds.URL,
		"VPC_PROOF_PROBES_ECHO_URLS":     echo.URL,
		"VPC_PROOF_PROBES_VPC_CIDR":      "192.168.0.0/16", // force a diagnostic hint
		"VPC_PROOF_LOG_FORMAT":           "console",
	}

	path := filepath.Join(t.TempDir(), "evidence.md")
	_, _, code := runCLI(t, env, "report", "--format", "markdown", "--output", path)
	if code != 0 {
		t.Fatalf("report exit code = %d, want 0", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	text := string(content)

	if !strings.Contains(text, "Integrity hash") {
		t.Error("report is missing the Integrity hash field")
	}
	if !regexp.MustCompile(`[0-9a-f]{64}`).MatchString(text) {
		t.Error("report integrity hash is not a 64-character hex digest")
	}
	if !strings.Contains(text, "Verify the instance's VPC") {
		t.Error("report is missing the VPC diagnostic hint")
	}
}

func TestAPIServerE2E(t *testing.T) {
	imds := mockIMDS(t, "203.0.113.7")
	echo := mockEcho(t, "203.0.113.7")
	port := freePort(t)

	env := map[string]string{
		"VPC_PROOF_SERVER_ADDR":                   "127.0.0.1",
		"VPC_PROOF_SERVER_PORT":                   fmt.Sprintf("%d", port),
		"VPC_PROOF_AUTH_ENABLED":                  "true",
		"VPC_PROOF_AUTH_TOKEN":                    "e2e-secret-token",
		"VPC_PROOF_RATELIMIT_REQUESTS_PER_MINUTE": "1000",
		"VPC_PROOF_PROBES_IMDS_BASE_URL":          imds.URL,
		"VPC_PROOF_PROBES_ECHO_URLS":              echo.URL,
	}

	cmd := exec.Command(binaryPath, "serve")
	cmd.Env = append(os.Environ(), envSlice(env)...)
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
	})

	waitForPort(t, port)

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 10 * time.Second}

	authenticate := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer e2e-secret-token")
		req.Header.Set("User-Agent", "e2e-client")
	}

	// Public health check.
	resp := doGet(t, client, base+"/healthz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Authentication is enforced.
	resp = doGet(t, client, base+"/api/v1/probe", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("probe without auth = %d, want 401", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Cached probe report.
	first := doAuthedGet(t, client, base+"/api/v1/probe", authenticate)
	second := doAuthedGet(t, client, base+"/api/v1/probe", authenticate)
	if first != second {
		t.Error("repeated probe requests returned different (uncached) bodies")
	}

	// Echo reflection.
	echoBody := doAuthedGet(t, client, base+"/api/v1/echo", authenticate)
	var echoed struct {
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
	}
	if err := json.Unmarshal([]byte(echoBody), &echoed); err != nil {
		t.Fatalf("decode echo: %v", err)
	}
	if echoed.IP == "" {
		t.Error("echo did not reflect the client IP")
	}
	if echoed.UserAgent != "e2e-client" {
		t.Errorf("echo user_agent = %q, want e2e-client", echoed.UserAgent)
	}

	// Graceful shutdown.
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal serve: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve did not exit cleanly: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not shut down within the timeout")
	}

	if !strings.Contains(logs.String(), "stopped gracefully") {
		t.Error("graceful shutdown log entry missing")
	}
}

func doGet(t *testing.T, client *http.Client, url string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if mutate != nil {
		mutate(req)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func doAuthedGet(t *testing.T, client *http.Client, url string, auth func(*http.Request)) string {
	t.Helper()
	resp := doGet(t, client, url, auth)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s = %d: %s", url, resp.StatusCode, body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}
