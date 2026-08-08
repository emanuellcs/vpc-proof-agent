// Package history tracks the state of the environment over time by keeping a
// capped, thread-safe list of probe run summaries. It optionally persists the
// list to disk with atomic (temp file + rename) writes.
package history

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

// defaultMaxEntries is the ring-buffer capacity when none is configured.
const defaultMaxEntries = 50

// Entry is a summary of a single probe run.
type Entry struct {
	// Timestamp is when the run started.
	Timestamp time.Time `json:"timestamp"`
	// Status is the overall run status.
	Status string `json:"status"`
	// Passed, Failed, Warned, Skipped are probe outcome counts.
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Warned  int `json:"warned"`
	Skipped int `json:"skipped"`
	// Duration is the total run duration.
	Duration time.Duration `json:"duration"`
}

// FromReport builds an Entry from a probe report.
func FromReport(report probe.Report) Entry {
	entry := Entry{
		Timestamp: report.StartedAt,
		Status:    report.Status.String(),
		Duration:  report.Duration,
	}
	for _, result := range report.Results {
		switch result.Status {
		case probe.StatusPass:
			entry.Passed++
		case probe.StatusFail:
			entry.Failed++
		case probe.StatusWarn:
			entry.Warned++
		case probe.StatusSkip:
			entry.Skipped++
		}
	}
	return entry
}

// Options configures the history store.
type Options struct {
	// MaxEntries is the ring-buffer capacity (default 50).
	MaxEntries int
	// DiskPath, when non-empty, enables persistence.
	DiskPath string
	// FlushInterval is how often Start flushes to disk (default 30s).
	FlushInterval time.Duration
	// Now is the injectable clock (default time.Now).
	Now func() time.Time
}

// Store is a thread-safe, capped list of run summaries. Reads take a read
// lock and writes take a write lock.
type Store struct {
	mu         sync.RWMutex
	entries    []Entry
	maxEntries int
	now        func() time.Time

	diskPath string
	flush    time.Duration
}

// New builds a Store from the given options, applying defaults.
func New(opts Options) *Store {
	maxEntries := opts.MaxEntries
	if maxEntries < 1 {
		maxEntries = defaultMaxEntries
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Store{
		maxEntries: maxEntries,
		now:        now,
		diskPath:   opts.DiskPath,
		flush:      opts.FlushInterval,
	}
}

// Append records a run summary, evicting the oldest entry when the store is
// full.
func (s *Store) Append(entry *Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append(s.entries, *entry)
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[len(s.entries)-s.maxEntries:]
	}
}

// List returns a copy of the entries, oldest first.
func (s *Store) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Len returns the number of stored entries.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// Start periodically flushes the store to disk until ctx is canceled. It is
// a no-op when no disk path is configured.
func (s *Store) Start(ctx context.Context) {
	if s.diskPath == "" {
		return
	}
	interval := s.flush
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.Flush()
		}
	}
}

// Flush writes the current entries to disk atomically: the data is written to
// a temporary file which is then renamed over the destination, so readers
// never observe a partially written file. The capped list bounds the file
// size, which acts as built-in rotation.
func (s *Store) Flush() error {
	if s.diskPath == "" {
		return nil
	}

	entries := s.List()
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("history: encode entries: %w", err)
	}

	tmp := s.diskPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("history: write temp file: %w", err)
	}
	if err := os.Rename(tmp, s.diskPath); err != nil {
		return fmt.Errorf("history: rename temp file: %w", err)
	}
	return nil
}

// Load restores entries from the disk file, if it exists. It is safe to call
// after New and before serving.
func (s *Store) Load() error {
	if s.diskPath == "" {
		return nil
	}
	raw, err := os.ReadFile(s.diskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("history: read disk file: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return fmt.Errorf("history: decode disk file: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = entries
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[len(s.entries)-s.maxEntries:]
	}
	return nil
}
