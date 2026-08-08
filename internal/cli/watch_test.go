package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/history"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
)

type staticProbe struct {
	id     string
	status probe.Status
}

func (s staticProbe) ID() string { return s.id }

func (s staticProbe) Execute(context.Context) probe.Result {
	return probe.Result{ID: s.id, Status: s.status, Message: s.id}
}

func testWatchApp() *App {
	return &App{
		runner: probe.NewRunner([]probe.Probe{
			staticProbe{id: "dns", status: probe.StatusPass},
		}),
		history: history.New(history.Options{}),
	}
}

func TestRunWatch(t *testing.T) {
	app := testWatchApp()

	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runWatch(ctx, app, 10*time.Millisecond, false, &out)
	}()

	// Let a couple of iterations run, then stop.
	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runWatch: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runWatch did not return after cancel")
	}

	if !strings.Contains(out.String(), "vpc-proof watch") {
		t.Errorf("output missing watch header:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "overall pass") {
		t.Errorf("output missing summary:\n%s", out.String())
	}
}

func TestRunWatchClearsScreen(t *testing.T) {
	app := testWatchApp()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runWatch(ctx, app, 10*time.Millisecond, true, &out)
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()
	<-done

	if !strings.Contains(out.String(), "\x1b[2J\x1b[H") {
		t.Errorf("clear sequence missing:\n%q", out.String())
	}
}

func TestPrintWatchStatus(t *testing.T) {
	var out bytes.Buffer
	report := probe.Report{
		Status: probe.StatusWarn,
		Results: []probe.Result{
			{ID: "dns", Status: probe.StatusPass},
			{ID: "clock_skew", Status: probe.StatusWarn},
		},
	}
	printWatchStatus(&out, report, false)

	if !strings.Contains(out.String(), "vpc-proof watch") {
		t.Errorf("header missing:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "clock_skew") {
		t.Errorf("probe result missing:\n%s", out.String())
	}
}
