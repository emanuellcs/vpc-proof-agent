package history

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

func testEntry(ts time.Time, status string) *Entry {
	return &Entry{Timestamp: ts, Status: status, Passed: 1, Duration: time.Second}
}

func TestAppendAndList(t *testing.T) {
	s := New(Options{})
	s.Append(testEntry(time.Now(), "pass"))
	s.Append(testEntry(time.Now(), "fail"))

	list := s.List()
	if s.Len() != 2 {
		t.Fatalf("Len = %d, want 2", s.Len())
	}
	if list[0].Status != "pass" || list[1].Status != "fail" {
		t.Errorf("entries out of order: %+v", list)
	}
}

func TestRingBufferEviction(t *testing.T) {
	s := New(Options{MaxEntries: 3})
	for range 5 {
		s.Append(testEntry(time.Now(), "pass"))
	}

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("list len = %d, want 3 (capped)", len(list))
	}
	if list[0].Status != "pass" {
		t.Error("oldest entry should have been evicted")
	}
	if s.Len() != 3 {
		t.Errorf("Len = %d, want 3", s.Len())
	}
}

func TestListReturnsCopy(t *testing.T) {
	s := New(Options{MaxEntries: 5})
	s.Append(testEntry(time.Now(), "pass"))

	list := s.List()
	list[0].Status = "tampered"

	if s.List()[0].Status != "pass" {
		t.Error("List should return a copy; mutating it must not affect the store")
	}
}

func TestFromReport(t *testing.T) {
	ts := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	report := probe.Report{
		StartedAt: ts,
		Duration:  3 * time.Second,
		Status:    probe.StatusWarn,
		Results: []probe.Result{
			{Status: probe.StatusPass},
			{Status: probe.StatusPass},
			{Status: probe.StatusFail},
			{Status: probe.StatusWarn},
			{Status: probe.StatusSkip},
		},
	}

	entry := FromReport(report)
	if entry.Timestamp != ts {
		t.Errorf("timestamp = %v", entry.Timestamp)
	}
	if entry.Status != "warn" || entry.Passed != 2 || entry.Failed != 1 || entry.Warned != 1 || entry.Skipped != 1 {
		t.Errorf("entry counts incorrect: %+v", entry)
	}
	if entry.Duration != 3*time.Second {
		t.Errorf("duration = %s", entry.Duration)
	}
}

func TestConcurrentAppend(t *testing.T) {
	const workers = 20
	const perWorker = 200
	total := workers * perWorker
	s := New(Options{MaxEntries: total})
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				s.Append(testEntry(time.Now(), "pass"))
			}
		}()
	}
	wg.Wait()

	if s.Len() != total {
		t.Fatalf("Len = %d, want %d (no lost updates)", s.Len(), workers*perWorker)
	}
}

func TestFlushAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s := New(Options{DiskPath: path})
	s.Append(testEntry(time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC), "pass"))

	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	restored := New(Options{DiskPath: path})
	if err := restored.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if restored.Len() != 1 {
		t.Fatalf("restored Len = %d, want 1", restored.Len())
	}
	if restored.List()[0].Status != "pass" {
		t.Errorf("restored entry = %+v", restored.List()[0])
	}
}

func TestFlushIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	s := New(Options{DiskPath: path})
	s.Append(testEntry(time.Now(), "pass"))

	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// No temporary file should remain after a successful flush.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temporary file should be renamed away, err = %v", err)
	}

	// The written file must be valid JSON.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("file is not valid JSON: %v", err)
	}
}

func TestFlushWithoutDiskPathIsNoop(t *testing.T) {
	s := New(Options{})
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush without disk path: %v", err)
	}
}

func TestFlushWriteError(t *testing.T) {
	// A directory as the target path makes the write fail.
	s := New(Options{DiskPath: t.TempDir()})
	s.Append(testEntry(time.Now(), "pass"))
	if err := s.Flush(); err == nil {
		t.Fatal("expected flush error for an invalid path, got nil")
	}
}

func TestLoadMissingFileIsNoop(t *testing.T) {
	s := New(Options{DiskPath: filepath.Join(t.TempDir(), "nope.json")})
	if err := s.Load(); err != nil {
		t.Fatalf("Load of a missing file should be a no-op: %v", err)
	}
	if s.Len() != 0 {
		t.Error("store should be empty")
	}
}

func TestLoadCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := New(Options{DiskPath: path})
	if err := s.Load(); err == nil {
		t.Fatal("expected error loading a corrupt file, got nil")
	}
}

func TestStartStopsOnCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s := New(Options{DiskPath: path, FlushInterval: 10 * time.Millisecond})
	s.Append(testEntry(time.Now(), "pass"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not stop after cancel")
	}
}

func TestStartWithoutDiskPathReturnsImmediately(t *testing.T) {
	s := New(Options{})
	s.Start(context.Background()) // must return immediately (no-op)
}

func TestStartWithoutDiskPathDoesNotTick(t *testing.T) {
	s := New(Options{FlushInterval: time.Millisecond})
	done := make(chan struct{})
	go func() {
		s.Start(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Start without a disk path should return immediately")
	}
}

func TestStartDefaultsFlushInterval(t *testing.T) {
	s := New(Options{DiskPath: filepath.Join(t.TempDir(), "history.json")})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start with a defaulted interval did not stop after cancel")
	}
}

func TestLoadFromDirectoryFails(t *testing.T) {
	s := New(Options{DiskPath: t.TempDir()})
	if err := s.Load(); err == nil {
		t.Fatal("expected an error when the disk path is a directory")
	}
}

func TestDefaultMaxEntries(t *testing.T) {
	s := New(Options{})
	if s.maxEntries != defaultMaxEntries {
		t.Errorf("default max = %d, want %d", s.maxEntries, defaultMaxEntries)
	}
}
