package probe

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
)

// Linux pseudo-file paths read by the system resources probe.
const (
	procUptimePath  = "/proc/uptime"
	procLoadavgPath = "/proc/loadavg"
	procMeminfoPath = "/proc/meminfo"
)

// System resource warning thresholds.
const (
	// memLowThresholdPercent warns when available memory drops below 10% of
	// the total.
	memLowThresholdPercent = 10.0
	// loadHighThreshold warns when the 1-minute load average exceeds 4.
	loadHighThreshold = 4.0
)

// systemResourcesHint is attached when the host metrics cannot be read.
const systemResourcesHint = "Unable to read host resource metrics (/proc/uptime, /proc/loadavg, /proc/meminfo)."

// SystemResourcesProbe inspects the host's system resources by parsing the
// Linux pseudo-files /proc/uptime, /proc/loadavg, and /proc/meminfo. The file
// reader is injectable so the probe is fully testable without a real host.
type SystemResourcesProbe struct {
	readFile func(string) ([]byte, error)
	logger   *observability.Logger
}

// NewSystemResourcesProbe builds a probe reading the given files. A nil
// readFile falls back to os.ReadFile.
func NewSystemResourcesProbe(readFile func(string) ([]byte, error), logger *observability.Logger) *SystemResourcesProbe {
	if readFile == nil {
		readFile = os.ReadFile
	}
	return &SystemResourcesProbe{readFile: readFile, logger: logger}
}

// ID returns the probe identifier.
func (p *SystemResourcesProbe) ID() string {
	return SystemResourcesProbeID
}

// Execute reads and parses the resource files, warning on low memory or high
// load.
func (p *SystemResourcesProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	result := Result{
		ID:      p.ID(),
		Name:    "System resources",
		Details: map[string]string{},
	}

	uptime, err := p.readUptime(ctx)
	if err != nil {
		return p.failed(&result, start, err)
	}
	loads, err := p.readLoadavg(ctx)
	if err != nil {
		return p.failed(&result, start, err)
	}
	memTotal, memAvailable, err := p.readMeminfo(ctx)
	if err != nil {
		return p.failed(&result, start, err)
	}

	result.Details["uptime_seconds"] = strconv.FormatFloat(uptime, 'f', 2, 64)
	result.Details["load1"] = strconv.FormatFloat(loads[0], 'f', 2, 64)
	result.Details["load5"] = strconv.FormatFloat(loads[1], 'f', 2, 64)
	result.Details["load15"] = strconv.FormatFloat(loads[2], 'f', 2, 64)
	result.Details["mem_total_kb"] = strconv.FormatUint(memTotal, 10)
	result.Details["mem_available_kb"] = strconv.FormatUint(memAvailable, 10)

	memUsedPercent := 0.0
	if memTotal > 0 {
		memUsedPercent = 100.0 * float64(memTotal-memAvailable) / float64(memTotal)
	}
	result.Details["mem_used_percent"] = strconv.FormatFloat(memUsedPercent, 'f', 1, 64)

	var warnings []string
	if memTotal > 0 && float64(memAvailable) < memLowThresholdPercent*float64(memTotal)/100.0 {
		warnings = append(warnings, fmt.Sprintf("critically low available memory (%.1f%%)", memUsedPercent))
	}
	if loads[0] > loadHighThreshold {
		warnings = append(warnings, fmt.Sprintf("excessively high system load (%.2f)", loads[0]))
	}

	result.Duration = time.Since(start)
	if len(warnings) > 0 {
		result.Status = StatusWarn
		result.Message = strings.Join(warnings, "; ")
		result.Hint = "Free up system resources or resize the instance if capacity is insufficient."
	} else {
		result.Status = StatusPass
		result.Message = "system resources within normal bounds"
	}
	return result
}

func (p *SystemResourcesProbe) failed(result *Result, start time.Time, err error) Result {
	result.Status = StatusFail
	result.Duration = time.Since(start)
	result.Message = fmt.Sprintf("could not read host resource metrics: %v", err)
	result.Hint = systemResourcesHint
	if p.logger != nil {
		p.logger.Debug("system resources probe failed", observability.Component("probe"), observability.Error(err))
	}
	return *result
}

// readUptime parses the first field of /proc/uptime (seconds since boot).
func (p *SystemResourcesProbe) readUptime(ctx context.Context) (float64, error) {
	data, err := p.readWithContext(ctx, procUptimePath)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("uptime: empty file")
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse uptime: %w", err)
	}
	return value, nil
}

// readLoadavg parses the 1, 5, and 15 minute load averages.
func (p *SystemResourcesProbe) readLoadavg(ctx context.Context) ([3]float64, error) {
	var loads [3]float64
	data, err := p.readWithContext(ctx, procLoadavgPath)
	if err != nil {
		return loads, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return loads, fmt.Errorf("loadavg: expected at least 3 fields, got %d", len(fields))
	}
	for i := range 3 {
		value, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return loads, fmt.Errorf("parse loadavg field %d: %w", i, err)
		}
		loads[i] = value
	}
	return loads, nil
}

// readMeminfo parses MemTotal and MemAvailable from /proc/meminfo, falling
// back to MemFree when MemAvailable is absent.
func (p *SystemResourcesProbe) readMeminfo(ctx context.Context) (total, available uint64, err error) {
	data, err := p.readWithContext(ctx, procMeminfoPath)
	if err != nil {
		return 0, 0, err
	}

	values := map[string]uint64{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 2)
		if len(parts) != 2 {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) == 0 {
			continue
		}
		value, parseErr := strconv.ParseUint(fields[0], 10, 64)
		if parseErr != nil {
			continue
		}
		values[strings.TrimSpace(parts[0])] = value
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("read meminfo: %w", err)
	}

	total = values["MemTotal"]
	available = values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"]
	}
	if total == 0 {
		return 0, 0, fmt.Errorf("meminfo: MemTotal missing")
	}
	return total, available, nil
}

// readWithContext reads a file, honoring context cancellation at the read
// boundaries.
func (p *SystemResourcesProbe) readWithContext(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := p.readFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
