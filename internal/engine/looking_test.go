package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/synth"
)

func TestWhatWasInFrontGoesWithTheTaskSoThisMeansSomething(t *testing.T) {
	steps := &recordingSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}}}
	e := &Engine{UIScreen: &movingScreen{}, Steps: steps}

	plan := drivePlan()
	plan.Drive.Looking = whatTheyWereLookingAt(&screen.Context{AppName: "Mail", WindowTitle: "Re: Friday plans"})
	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	task := steps.requests[0].Task
	if !strings.Contains(task, "say hello") || !strings.Contains(task, `looking at Mail, in the window "Re: Friday plans"`) {
		t.Errorf("the model was given %q, want the task and what was in front when it was asked", task)
	}
	if plan.Drive.Task != "say hello" {
		t.Errorf("the task itself became %q; what was in front is context, not part of what was asked", plan.Drive.Task)
	}
}

func TestNothingIsSaidAboutTheScreenWhenItIsNotKnown(t *testing.T) {
	for _, at := range []*screen.Context{nil, {}, {AppName: "vans"}} {
		if note := whatTheyWereLookingAt(at); note != "" {
			t.Errorf("said %q about a screen nobody read", note)
		}
	}
	if got := taskWithLooking(context.Background(), "say hello"); got != "say hello" {
		t.Errorf("task = %q", got)
	}
}
