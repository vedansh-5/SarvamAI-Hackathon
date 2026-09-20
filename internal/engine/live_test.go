//go:build integration

package engine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func liveApp(t *testing.T) (string, context.Context) {
	t.Helper()
	if os.Getenv("VANS_LIVE") == "" {
		t.Skip("set VANS_LIVE=1 to drive a real window")
	}
	app := os.Getenv("VANS_LIVE_APP")
	if app == "" {
		t.Skip("set VANS_LIVE_APP to the application to drive, e.g. VANS_LIVE_APP=Notes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	return app, ctx
}

func TestLiveDriveTask(t *testing.T) {
	app, ctx := liveApp(t)
	if os.Getenv("VANS_LIVE_DRIVE_OK") == "" {
		t.Skip("set VANS_LIVE_DRIVE_OK=1 to run a live driving task on screen")
	}

	task := os.Getenv("VANS_LIVE_TASK")
	if task == "" {
		task = "inspect the window and finish"
	}

	runner := appleevents.Osascript{Timeout: 120 * time.Second}
	screen := uiact.System{Runner: runner}

	tree, err := screen.Capture(ctx, app)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if len(tree.Elements) == 0 {
		t.Fatalf("%s has no controls on screen", app)
	}

	eng := &Engine{
		UIScreen: screen,
		Steps: &scriptedSteps{
			answers: []synth.StepAnswer{
				{Done: true, Why: "completed live inspection of " + app},
			},
		},
		Log: func(format string, args ...any) {
			t.Logf(format, args...)
		},
	}

	plan := &Plan{
		Drive: &DrivePlan{
			App:  app,
			Task: task,
		},
	}

	receipt, err := eng.Commit(ctx, plan)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !receipt.Verified {
		t.Errorf("receipt.Verified = false, want true")
	}
	t.Logf("receipt: title=%s verified=%v", receipt.Title, receipt.Verified)
}
