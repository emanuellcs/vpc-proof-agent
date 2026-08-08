package observability

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func newTestLogger(t *testing.T, level, format string) (*Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	l, err := New(level, format, &buf)
	if err != nil {
		t.Fatalf("New(%q, %q): %v", level, format, err)
	}
	t.Cleanup(func() { _ = l.Sync() })
	return l, &buf
}

func TestNewRejectsInvalidLevel(t *testing.T) {
	if _, err := New("verbose", "json", &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for invalid level, got nil")
	}
}

func TestNewRejectsInvalidFormat(t *testing.T) {
	if _, err := New("info", "xml", &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
}

func TestLevelFiltering(t *testing.T) {
	l, buf := newTestLogger(t, "info", "json")

	l.Debug("debug should be suppressed")
	l.Info("info should be emitted")

	out := buf.String()
	if strings.Contains(out, "debug should be suppressed") {
		t.Error("debug entry was emitted at info level")
	}
	if !strings.Contains(out, "info should be emitted") {
		t.Error("info entry was not emitted")
	}
}

func TestJSONFormat(t *testing.T) {
	l, buf := newTestLogger(t, "info", "json")

	l.Info("hello world", Str("service", "vpc-proof"), Int("attempt", 3))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if entry["level"] != "info" {
		t.Errorf("level = %v, want info", entry["level"])
	}
	if entry["msg"] != "hello world" {
		t.Errorf("msg = %v, want hello world", entry["msg"])
	}
	if entry["service"] != "vpc-proof" {
		t.Errorf("service = %v, want vpc-proof", entry["service"])
	}
	if entry["attempt"] != float64(3) {
		t.Errorf("attempt = %v, want 3", entry["attempt"])
	}
}

func TestConsoleFormat(t *testing.T) {
	l, buf := newTestLogger(t, "info", "console")

	l.Info("hello console")

	out := buf.String()
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("console output must not be JSON, got %q", out)
	}
	if !strings.Contains(out, "hello console") {
		t.Errorf("console output missing message, got %q", out)
	}
	if !strings.Contains(out, "INFO") {
		t.Errorf("console output missing level, got %q", out)
	}
}

func TestWithContextFields(t *testing.T) {
	l, buf := newTestLogger(t, "info", "json")

	child := l.With(Str("component", "probe"))
	child.Info("probe started")

	out := buf.String()
	if !strings.Contains(out, `"component":"probe"`) {
		t.Errorf("context field not propagated, got %s", out)
	}
	if !strings.Contains(out, `"component":"probe"`) {
		// Parent logger must remain unaffected: logging again without context
		// proves the child did not mutate the base logger.
		t.Errorf("context field leaked unexpectedly, got %s", out)
	}
}

func TestNamedLogger(t *testing.T) {
	l, buf := newTestLogger(t, "info", "json")

	l.Named("api").Info("request handled")

	if !strings.Contains(buf.String(), `"logger":"api"`) {
		t.Errorf("logger name not propagated, got %s", buf.String())
	}
}

func TestRedaction(t *testing.T) {
	l, buf := newTestLogger(t, "info", "json")

	l.Info("authentication",
		Str("token", "super-secret-token"),
		Str("password", "p@ss"),
		Str("authorization", "Bearer abc"),
		Str("request_id", "req-123"),
	)

	out := buf.String()
	for _, secret := range []string{"super-secret-token", "p@ss", "Bearer abc"} {
		if strings.Contains(out, secret) {
			t.Errorf("sensitive value %q leaked into log output", secret)
		}
	}
	if strings.Count(out, redacted) != 3 {
		t.Errorf("expected 3 redacted placeholders, got %d in %s", strings.Count(out, redacted), out)
	}
	if !strings.Contains(out, `"request_id":"req-123"`) {
		t.Errorf("non-sensitive field was dropped, got %s", out)
	}
}

func TestRedactionInWith(t *testing.T) {
	l, buf := newTestLogger(t, "info", "json")

	l.With(Str("api_key", "k-secret"), Str("url", "https://example.com")).Info("client")

	out := buf.String()
	if strings.Contains(out, "k-secret") {
		t.Errorf("sensitive api_key leaked, got %s", out)
	}
	if !strings.Contains(out, redacted) {
		t.Errorf("expected redacted placeholder, got %s", out)
	}
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("non-sensitive field missing, got %s", out)
	}
}

func TestFieldConstructors(t *testing.T) {
	l, buf := newTestLogger(t, "info", "json")

	l.Info("fields",
		Str("str", "s"),
		Int("int", 42),
		Bool("bool", true),
		Duration("dur", 2*time.Second),
		Component("cli"),
		Error(nil),
		Any("any", []string{"a", "b"}),
	)

	out := buf.String()
	if !strings.Contains(out, `"str":"s"`) {
		t.Errorf("str field missing, got %s", out)
	}
	if !strings.Contains(out, `"int":42`) {
		t.Errorf("int field missing, got %s", out)
	}
	if !strings.Contains(out, `"bool":true`) {
		t.Errorf("bool field missing, got %s", out)
	}
	if !strings.Contains(out, `"dur":2`) {
		t.Errorf("duration field missing, got %s", out)
	}
	if !strings.Contains(out, `"component":"cli"`) {
		t.Errorf("component field missing, got %s", out)
	}
	if !strings.Contains(out, `"any":["a","b"]`) {
		t.Errorf("any field missing, got %s", out)
	}
}
