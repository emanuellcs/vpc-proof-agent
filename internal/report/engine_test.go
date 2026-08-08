package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/diagnostic"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func testData() *Data {
	startedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	data := &Data{
		Agent: AgentInfo{
			Name:      "vpc-proof",
			Version:   "1.2.3",
			Commit:    "abc1234",
			BuildDate: "2026-08-08T12:00:00Z",
			GoVersion: "go1.26.5",
			Platform:  "linux/amd64",
		},
		Instance: Instance{
			InstanceID:       "i-0123456789abcdef0",
			AvailabilityZone: "us-east-1a",
			PrivateIP:        "10.0.1.42",
			PublicIP:         "203.0.113.7",
			MACAddress:       "0a:1b:2c:3d:4e:5f",
			SubnetCIDR:       "10.0.1.0/24",
			VpcCIDR:          "10.0.0.0/16",
		},
		Network: Network{
			DefaultGateway:   "10.0.2.2",
			DefaultInterface: "eth0",
			PrimaryIP:        "10.0.1.42",
			DNSAddresses:     "203.0.113.10, 203.0.113.11",
		},
		Summary: Summary{
			Status: "warn", Passed: 5, Failed: 1, Warned: 1, Skipped: 0,
			Total: 7, Duration: 3 * time.Second,
		},
		Probes: probe.Report{
			StartedAt: startedAt,
			Duration:  3 * time.Second,
			Status:    probe.StatusWarn,
			Results: []probe.Result{
				{ID: probe.MetadataProbeID, Name: "IMDSv2 metadata", Status: probe.StatusPass, Duration: 10 * time.Millisecond, Message: "IMDSv2 metadata accessible", Details: map[string]string{"instance_id": "i-0123456789abcdef0"}},
				{ID: probe.VPCOwnershipProbeID, Name: "VPC ownership", Status: probe.StatusPass, Duration: 20 * time.Millisecond, Message: "private IP 10.0.1.42 is inside VPC 10.0.0.0/16"},
				{ID: probe.SubnetOwnershipProbeID, Name: "Subnet ownership", Status: probe.StatusFail, Duration: 30 * time.Millisecond, Message: "private IP 10.0.1.42 is not inside subnet 10.0.2.0/24", Hint: "Verify the EC2 instance was launched in the correct Subnet (10.0.1.0/24)."},
				{ID: probe.DefaultRouteProbeID, Name: "Default route", Status: probe.StatusPass, Duration: 40 * time.Millisecond, Message: "default route via eth0 (gateway 10.0.2.2)"},
				{ID: probe.DNSProbeID, Name: "DNS resolution", Status: probe.StatusPass, Duration: 50 * time.Millisecond, Message: "resolved amazon.com", Details: map[string]string{"host": "amazon.com", "addresses": "203.0.113.10, 203.0.113.11"}},
				{ID: probe.InternetHTTPSProbeID, Name: "Outbound HTTPS", Status: probe.StatusPass, Duration: 60 * time.Millisecond, Message: "outbound HTTPS to https://checkip.amazonaws.com succeeded"},
				{ID: probe.PublicIPConsistencyProbeID, Name: "Public IP consistency", Status: probe.StatusWarn, Duration: 70 * time.Millisecond, Message: "instance has no public IP assigned"},
			},
		},
		Diagnostics: []diagnostic.Hint{
			{RuleID: "subnet-placement", Severity: diagnostic.SeverityCritical, Message: "Verify the EC2 instance was launched in the correct Subnet (10.0.1.0/24)."},
			{RuleID: "public-ip-assignment", Severity: diagnostic.SeverityWarning, Message: "Ensure the Subnet has 'Auto-assign public IP' enabled and the instance has a public IP associated."},
		},
		GeneratedAt: startedAt,
	}
	data.SetIntegrityHash()
	return data
}

func TestIntegrityHash(t *testing.T) {
	pr := probe.Report{
		StartedAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
		Status:    probe.StatusPass,
		Results: []probe.Result{
			{ID: probe.MetadataProbeID, Status: probe.StatusPass, Details: map[string]string{"instance_id": "i-1"}},
		},
	}
	agent := AgentInfo{Version: "1.0.0"}

	data := Build(pr, nil, &agent)
	if data.IntegrityHash == "" {
		t.Fatal("Build should populate the integrity hash")
	}
	if !VerifyIntegrityHash(&data) {
		t.Error("stored hash should verify against a fresh computation")
	}

	// Tampering with any field must change the hash.
	tampered := data
	tampered.IntegrityHash = ""
	tampered.Instance.PrivateIP = "10.0.9.9"
	if VerifyIntegrityHash(&tampered) {
		t.Error("tampered report should fail verification")
	}
}

func TestIntegrityHashDeterministic(t *testing.T) {
	startedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	makeData := func(details map[string]string) Data {
		pr := probe.Report{
			StartedAt: startedAt,
			Status:    probe.StatusPass,
			Results: []probe.Result{
				{ID: probe.MetadataProbeID, Status: probe.StatusPass, Details: details},
			},
		}
		agent := AgentInfo{Version: "1.0.0"}
		return Build(pr, nil, &agent)
	}

	// Identical content with maps inserted in different orders must hash to
	// the same value.
	a := makeData(map[string]string{"b": "1", "a": "2", "c": "3"})
	b := makeData(map[string]string{"c": "3", "a": "2", "b": "1"})

	if a.IntegrityHash != b.IntegrityHash {
		t.Errorf("hash is not deterministic across map ordering:\n a=%s\n b=%s", a.IntegrityHash, b.IntegrityHash)
	}
	if !VerifyIntegrityHash(&a) || !VerifyIntegrityHash(&b) {
		t.Error("both variants should verify")
	}
}

func TestGolden(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	data := testData()

	tests := []struct {
		name   string
		format Format
		golden string
	}{
		{name: "json", format: FormatJSON, golden: "testdata/golden_report.json"},
		{name: "markdown", format: FormatMarkdown, golden: "testdata/golden_report.md"},
		{name: "text", format: FormatText, golden: "testdata/golden_report.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.Render(data, tt.format)
			if err != nil {
				t.Fatalf("Render(%s): %v", tt.format, err)
			}

			if *updateGolden {
				if writeErr := os.WriteFile(tt.golden, got, 0o644); writeErr != nil {
					t.Fatalf("update golden: %v", writeErr)
				}
				return
			}

			want, err := os.ReadFile(tt.golden)
			if err != nil {
				t.Fatalf("read golden: %v (run with -update to generate)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s output differs from golden\n got:\n%s\nwant:\n%s", tt.format, got, want)
			}
		})
	}
}

func TestJSONSchema(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rendered, err := engine.Render(testData(), FormatJSON)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(rendered, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	agent, ok := doc["agent"].(map[string]any)
	if !ok || agent["version"] != "1.2.3" {
		t.Errorf("agent section incorrect: %v", doc["agent"])
	}

	instance, ok := doc["instance"].(map[string]any)
	if !ok || instance["instance_id"] != "i-0123456789abcdef0" {
		t.Errorf("instance section incorrect: %v", doc["instance"])
	}

	summary, ok := doc["summary"].(map[string]any)
	if !ok || summary["status"] != "warn" || summary["passed"] != float64(5) {
		t.Errorf("summary section incorrect: %v", doc["summary"])
	}

	probes, ok := doc["probes"].(map[string]any)
	if !ok {
		t.Fatalf("probes section missing: %v", doc)
	}
	if probes["status"] != "warn" {
		t.Errorf("probes status should be a string, got %v", probes["status"])
	}

	results, ok := probes["results"].([]any)
	if !ok || len(results) != 7 {
		t.Fatalf("probes results incorrect: %v", probes["results"])
	}
	first, ok := results[0].(map[string]any)
	if !ok || first["status"] != "pass" {
		t.Errorf("first result status should be a string, got %v", first)
	}

	diagnostics, ok := doc["diagnostics"].([]any)
	if !ok || len(diagnostics) != 2 {
		t.Errorf("diagnostics incorrect: %v", doc["diagnostics"])
	}
}

func TestRenderMarkdownContent(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rendered, err := engine.Render(testData(), FormatMarkdown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := string(rendered)
	for _, want := range []string{
		"# VPC Proof Agent — Evidence Report",
		"| Instance ID | i-0123456789abcdef0 |",
		"| Overall status | warn |",
		"## 4. Probe Results",
		"| subnet_ownership | fail |",
		"## 5. Diagnostics",
		"- **critical**: Verify the EC2 instance was launched in the correct Subnet (10.0.1.0/24).",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown output missing %q", want)
		}
	}
}

func TestRenderTextContent(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rendered, err := engine.Render(testData(), FormatText)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := string(rendered)
	for _, want := range []string{
		"VPC PROOF AGENT — EVIDENCE REPORT",
		"Instance ID         : i-0123456789abcdef0",
		"subnet_ownership     fail",
		"- [critical] Verify the EC2 instance was launched in the correct Subnet (10.0.1.0/24).",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q", want)
		}
	}
}

func TestRenderUnavailableFields(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	data := testData()
	data.Instance = Instance{}

	rendered, err := engine.Render(data, FormatMarkdown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(rendered), "| Instance ID | N/A |") {
		t.Errorf("markdown should render N/A for unavailable fields:\n%s", rendered)
	}

	renderedText, err := engine.Render(data, FormatText)
	if err != nil {
		t.Fatalf("Render text: %v", err)
	}
	if !strings.Contains(string(renderedText), "Instance ID         : N/A") {
		t.Errorf("text should render N/A for unavailable fields:\n%s", renderedText)
	}
}

func TestRenderNoDiagnostics(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	data := testData()
	data.Diagnostics = nil

	rendered, err := engine.Render(data, FormatMarkdown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(rendered), "No issues detected.") {
		t.Errorf("markdown should indicate no issues, got:\n%s", rendered)
	}
}

func TestRenderInvalidFormat(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := engine.Render(testData(), Format("xml")); err == nil {
		t.Fatal("expected error for unsupported format, got nil")
	}
}

func TestWrite(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var buf bytes.Buffer
	if err := engine.Write(&buf, testData(), FormatMarkdown); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(buf.String(), "VPC Proof Agent — Evidence Report") {
		t.Error("Write output missing title")
	}
}

func TestWriteFile(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	path := filepath.Join(t.TempDir(), "report.md")
	if writeErr := engine.WriteFile(path, testData(), FormatMarkdown); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("file permissions = %o, want 0644", perm)
	}

	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if !strings.Contains(string(content), "VPC Proof Agent — Evidence Report") {
		t.Error("file content missing title")
	}

	// Writing again must truncate, not append.
	if writeErr := engine.WriteFile(path, testData(), FormatJSON); writeErr != nil {
		t.Fatalf("second WriteFile: %v", writeErr)
	}
	content, readErr = os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read after rewrite: %v", readErr)
	}
	if strings.Contains(string(content), "Evidence Report") {
		t.Error("file should have been truncated and rewritten as JSON")
	}
	var doc map[string]any
	if unmarshalErr := json.Unmarshal(content, &doc); unmarshalErr != nil {
		t.Errorf("rewritten file is not valid JSON: %v", unmarshalErr)
	}
}

func TestWriteFileInvalidPath(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = engine.WriteFile(filepath.Join(t.TempDir(), "no-such-dir", "report.md"), testData(), FormatText)
	if err == nil {
		t.Fatal("expected error writing to an invalid path, got nil")
	}
}

func TestParseFormat(t *testing.T) {
	for _, in := range []string{"json", "markdown", "text"} {
		got, err := ParseFormat(in)
		if err != nil {
			t.Errorf("ParseFormat(%q): %v", in, err)
		}
		if got.String() != in {
			t.Errorf("ParseFormat(%q) = %q", in, got)
		}
	}

	if _, err := ParseFormat("xml"); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestNewEngineParseError(t *testing.T) {
	if _, err := newEngine(fstest.MapFS{}); err == nil {
		t.Fatal("expected error parsing templates from an empty filesystem, got nil")
	}
}

func TestRenderTemplateExecuteError(t *testing.T) {
	broken := fstest.MapFS{
		"templates/markdown.md.tmpl": {Data: []byte("{{ .MissingField }}")},
		"templates/text.txt.tmpl":    {Data: []byte("ok")},
	}
	engine, err := newEngine(broken)
	if err != nil {
		t.Fatalf("newEngine: %v", err)
	}

	if _, err := engine.Render(&Data{}, FormatMarkdown); err == nil {
		t.Fatal("expected execute error for a template referencing a missing field, got nil")
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestWriteError(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := engine.Write(errorWriter{}, testData(), FormatMarkdown); err == nil {
		t.Fatal("expected write error, got nil")
	}
}

func TestWriteFileWriteError(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("/dev/full not available on this platform")
	}
	engine, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := engine.WriteFile("/dev/full", testData(), FormatMarkdown); err == nil {
		t.Fatal("expected write error, got nil")
	}
}
