package cli

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emanuellcs/vpc-proof-agent/pkg/netutil"
)

func TestStatusCommand(t *testing.T) {
	deps, _ := defaultDeps(t)
	stdout, _, code := runCLIWith(&deps, "status")

	if code != exitCodeOK {
		t.Fatalf("status exit code = %d, want 0", code)
	}
	for _, want := range []string{
		"Instance ID",
		"i-0123456789abcdef0",
		"us-east-1a",
		"10.0.1.42",
		"203.0.113.7",
		"present (gateway 10.0.2.2 via eth0)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status output missing %q, got:\n%s", want, stdout)
		}
	}
}

func TestStatusMetadataUnavailable(t *testing.T) {
	deps := appDeps{
		metadataClient: fakeMetadata{err: errors.New("imds unreachable")},
		routeReader: fakeRouteReader{routes: []netutil.Route{
			{Interface: "eth0", Destination: netip.IPv4Unspecified(), Gateway: netip.MustParseAddr("10.0.2.2"), Flags: 0x3},
		}},
	}

	stdout, _, code := runCLIWith(&deps, "status")
	if code != exitCodeOK {
		t.Fatalf("status exit code = %d, want 0", code)
	}
	if strings.Count(stdout, "unavailable") < 4 {
		t.Errorf("metadata fields should be unavailable, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "present (gateway 10.0.2.2 via eth0)") {
		t.Errorf("default route should still be shown, got:\n%s", stdout)
	}
}

func TestStatusPublicIPNone(t *testing.T) {
	deps := appDeps{
		metadataClient: fakeMetadata{
			instanceID: "i-0123456789abcdef0",
			privateIP:  "10.0.1.42",
			az:         "us-east-1a",
		},
		routeReader: fakeRouteReader{routes: []netutil.Route{
			{Interface: "eth0", Destination: netip.IPv4Unspecified(), Gateway: netip.MustParseAddr("10.0.2.2"), Flags: 0x3},
		}},
	}

	stdout, _, code := runCLIWith(&deps, "status")
	if code != exitCodeOK {
		t.Fatalf("status exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Public IP") || !strings.Contains(stdout, "none") {
		t.Errorf("public IP should show none, got:\n%s", stdout)
	}
}

func TestCheckExitCodePass(t *testing.T) {
	deps, server := defaultDeps(t)
	t.Setenv("VPC_PROOF_PROBES_ECHO_URLS", server.URL)

	stdout, _, code := runCLIWith(&deps, "check")
	if code != exitCodeOK {
		t.Fatalf("check exit code = %d, want 0 (all pass)", code)
	}
	if !strings.Contains(stdout, "overall pass") {
		t.Errorf("summary should report overall pass, got:\n%s", stdout)
	}
}

func TestCheckExitCodeFail(t *testing.T) {
	deps, server := defaultDeps(t)
	t.Setenv("VPC_PROOF_PROBES_ECHO_URLS", server.URL)
	// Move the instance outside the expected VPC/subnet so ownership fails.
	meta := deps.metadataClient.(fakeMetadata)
	meta.privateIP = "192.168.1.5"
	deps.metadataClient = meta

	_, stderr, code := runCLIWith(&deps, "check")
	if code != exitCodeFailure {
		t.Fatalf("check exit code = %d, want %d (failures)", code, exitCodeFailure)
	}
	if !strings.Contains(stderr, "probe(s) failed") {
		t.Errorf("stderr should mention failures, got %q", stderr)
	}
}

func TestCheckExitCodeWarn(t *testing.T) {
	deps, server := defaultDeps(t)
	t.Setenv("VPC_PROOF_PROBES_ECHO_URLS", server.URL)
	// No public IP assigned => the consistency probe warns (nothing fails).
	meta := deps.metadataClient.(fakeMetadata)
	meta.publicIP = ""
	deps.metadataClient = meta

	_, stderr, code := runCLIWith(&deps, "check")
	if code != exitCodeWarn {
		t.Fatalf("check exit code = %d, want %d (warnings)", code, exitCodeWarn)
	}
	if !strings.Contains(stderr, "warnings") {
		t.Errorf("stderr should mention warnings, got %q", stderr)
	}
}

func TestDiagnosePrintsHints(t *testing.T) {
	deps, server := defaultDeps(t)
	t.Setenv("VPC_PROOF_PROBES_ECHO_URLS", server.URL)
	meta := deps.metadataClient.(fakeMetadata)
	meta.privateIP = "192.168.1.5"
	deps.metadataClient = meta

	stdout, _, code := runCLIWith(&deps, "diagnose")
	if code != exitCodeOK {
		t.Fatalf("diagnose exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Troubleshooting hints:") {
		t.Errorf("diagnose should print hints, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "correct Subnet (10.0.1.0/24)") {
		t.Errorf("diagnose should include the subnet hint, got:\n%s", stdout)
	}
}

func TestDiagnoseNoIssues(t *testing.T) {
	deps, server := defaultDeps(t)
	t.Setenv("VPC_PROOF_PROBES_ECHO_URLS", server.URL)

	stdout, _, code := runCLIWith(&deps, "diagnose")
	if code != exitCodeOK {
		t.Fatalf("diagnose exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "No issues detected.") {
		t.Errorf("diagnose should report no issues, got:\n%s", stdout)
	}
}

func TestReportToStdout(t *testing.T) {
	deps, server := defaultDeps(t)
	t.Setenv("VPC_PROOF_PROBES_ECHO_URLS", server.URL)

	stdout, _, code := runCLIWith(&deps, "report", "--format", "json", "--output", "-")
	if code != exitCodeOK {
		t.Fatalf("report exit code = %d, want 0", code)
	}
	for _, want := range []string{`"agent"`, `"probes"`, `"diagnostics"`, `"instance"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report output missing %q", want)
		}
	}
	if !strings.Contains(stdout, `"instance_id": "i-0123456789abcdef0"`) {
		t.Errorf("report should include instance id, got:\n%s", stdout)
	}
}

func TestReportToFile(t *testing.T) {
	deps, server := defaultDeps(t)
	t.Setenv("VPC_PROOF_PROBES_ECHO_URLS", server.URL)

	path := filepath.Join(t.TempDir(), "evidence.md")
	stdout, _, code := runCLIWith(&deps, "report", "--format", "markdown", "--output", path)
	if code != exitCodeOK {
		t.Fatalf("report exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "report written to") {
		t.Errorf("stdout should confirm the write, got %q", stdout)
	}
	if !strings.Contains(stdout, path) {
		t.Errorf("stdout should contain the path, got %q", stdout)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}
	if !strings.Contains(string(content), "VPC Proof Agent: Evidence Report") {
		t.Errorf("report file missing title, got:\n%s", content)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("report file permissions = %v, want 0644", info.Mode().Perm())
	}
}
