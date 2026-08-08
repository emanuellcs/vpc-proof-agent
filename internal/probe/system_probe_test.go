package probe

import (
	"context"
	"fmt"
	"testing"
)

// fakeProcFS serves canned /proc file content by path.
type fakeProcFS struct {
	files map[string]string
}

func (f fakeProcFS) ReadFile(path string) ([]byte, error) {
	content, ok := f.files[path]
	if !ok {
		return nil, fmt.Errorf("no such file %s", path)
	}
	return []byte(content), nil
}

const (
	healthyUptime  = "12345.67 43210.98\n"
	healthyLoadavg = "0.50 0.30 0.20 1/234 567\n"
	healthyMeminfo = "MemTotal:       1000000 kB\nMemFree:         950000 kB\nMemAvailable:    950000 kB\n"
)

func healthyProcFS() fakeProcFS {
	return fakeProcFS{files: map[string]string{
		procUptimePath:  healthyUptime,
		procLoadavgPath: healthyLoadavg,
		procMeminfoPath: healthyMeminfo,
	}}
}

func TestSystemResourcesProbePass(t *testing.T) {
	p := NewSystemResourcesProbe(healthyProcFS().ReadFile, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusPass {
		t.Fatalf("status = %s, want pass: %s", result.Status, result.Message)
	}
	if result.Details["uptime_seconds"] != "12345.67" {
		t.Errorf("uptime detail = %q", result.Details["uptime_seconds"])
	}
	if result.Details["load1"] != "0.50" {
		t.Errorf("load1 detail = %q", result.Details["load1"])
	}
	if result.Details["mem_total_kb"] != "1000000" {
		t.Errorf("mem_total detail = %q", result.Details["mem_total_kb"])
	}
	if result.Details["mem_available_kb"] != "950000" {
		t.Errorf("mem_available detail = %q", result.Details["mem_available_kb"])
	}
}

func TestSystemResourcesProbeWarnsOnLowMemory(t *testing.T) {
	fs := healthyProcFS()
	fs.files[procMeminfoPath] = "MemTotal:       1000000 kB\nMemFree:          50000 kB\nMemAvailable:     50000 kB\n"
	p := NewSystemResourcesProbe(fs.ReadFile, nil)

	result := p.Execute(context.Background())
	if result.Status != StatusWarn {
		t.Fatalf("status = %s, want warn: %s", result.Status, result.Message)
	}
	if result.Hint == "" {
		t.Error("warn result should carry a hint")
	}
	if result.Details["mem_used_percent"] != "95.0" {
		t.Errorf("mem_used_percent = %q, want 95.0", result.Details["mem_used_percent"])
	}
}

func TestSystemResourcesProbeWarnsOnHighLoad(t *testing.T) {
	fs := healthyProcFS()
	fs.files[procLoadavgPath] = "8.50 6.00 4.00 3/512 1000\n"
	p := NewSystemResourcesProbe(fs.ReadFile, nil)

	result := p.Execute(context.Background())
	if result.Status != StatusWarn {
		t.Fatalf("status = %s, want warn: %s", result.Status, result.Message)
	}
}

func TestSystemResourcesProbeWarnsOnBoth(t *testing.T) {
	fs := healthyProcFS()
	fs.files[procLoadavgPath] = "9.00 6.00 4.00 3/512 1000\n"
	fs.files[procMeminfoPath] = "MemTotal:       1000000 kB\nMemAvailable:     30000 kB\n"
	p := NewSystemResourcesProbe(fs.ReadFile, nil)

	result := p.Execute(context.Background())
	if result.Status != StatusWarn {
		t.Fatalf("status = %s, want warn", result.Status)
	}
}

func TestSystemResourcesProbeFallsBackToMemFree(t *testing.T) {
	fs := healthyProcFS()
	fs.files[procMeminfoPath] = "MemTotal:       1000000 kB\nMemFree:         900000 kB\n"
	p := NewSystemResourcesProbe(fs.ReadFile, nil)

	result := p.Execute(context.Background())
	if result.Status != StatusPass {
		t.Fatalf("status = %s, want pass (MemFree fallback)", result.Status)
	}
	if result.Details["mem_available_kb"] != "900000" {
		t.Errorf("mem_available = %q, want 900000", result.Details["mem_available_kb"])
	}
}

func TestSystemResourcesProbeReadFailure(t *testing.T) {
	// Drop the loadavg file so the probe fails.
	fs := healthyProcFS()
	delete(fs.files, procLoadavgPath)
	p := NewSystemResourcesProbe(fs.ReadFile, nil)
	result := p.Execute(context.Background())

	if result.Status != StatusFail {
		t.Fatalf("status = %s, want fail", result.Status)
	}
	if result.Hint != systemResourcesHint {
		t.Errorf("hint = %q, want system resources hint", result.Hint)
	}
}

func TestSystemResourcesProbeParseFailures(t *testing.T) {
	tests := []struct {
		name string
		path string
		data string
	}{
		{name: "bad uptime", path: procUptimePath, data: "not-a-number\n"},
		{name: "empty uptime", path: procUptimePath, data: "\n"},
		{name: "short loadavg", path: procLoadavgPath, data: "1.0\n"},
		{name: "bad loadavg", path: procLoadavgPath, data: "a b c\n"},
		{name: "missing memtotal", path: procMeminfoPath, data: "MemFree: 100 kB\n"},
		{name: "empty meminfo", path: procMeminfoPath, data: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := healthyProcFS()
			fs.files[tt.path] = tt.data
			p := NewSystemResourcesProbe(fs.ReadFile, nil)

			result := p.Execute(context.Background())
			if result.Status != StatusFail {
				t.Fatalf("status = %s, want fail", result.Status)
			}
		})
	}
}

func TestSystemResourcesProbeCanceled(t *testing.T) {
	p := NewSystemResourcesProbe(healthyProcFS().ReadFile, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := p.Execute(ctx)
	if result.Status != StatusFail {
		t.Fatalf("status = %s, want fail on canceled context", result.Status)
	}
}
