package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type keepingScreen struct {
	movingScreen
	held    map[string]string
	putBack []string
}

func (k *keepingScreen) ValueOf(_ context.Context, _ uiact.Tree, sel uiact.Selector) (string, bool) {
	v, ok := k.held[sel.Title]
	return v, ok
}

func (k *keepingScreen) PutBack(_ context.Context, app string, sel uiact.Selector, value string) error {
	k.putBack = append(k.putBack, app+"/"+sel.Title+"="+value)
	return nil
}

func typeTwice() []synth.StepAnswer {
	second := typeIntoMessage()
	second.Text = "hello again"
	second.Describe = "Type hello again"
	return []synth.StepAnswer{{Step: typeIntoMessage()}, {Step: second}, {Done: true, Why: "typed"}}
}

func TestWhatAFieldHeldIsKeptOnceAndPutBack(t *testing.T) {
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })

	screen := &keepingScreen{held: map[string]string{"Message": "a draft I was writing"}}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{answers: typeTwice()}}
	plan := &Plan{Drive: &DrivePlan{App: "Editor", Task: "say hello"}}

	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if got := plan.Drive.typed; len(got) != 1 || got[0].Was != "a draft I was writing" || got[0].App != "Editor" {
		t.Fatalf("typed = %+v, want the one field with what it held before the first keystroke", got)
	}

	how := undoFor(plan)
	if !strings.Contains(how, `"kind":"typing"`) {
		t.Fatalf("undo = %q, want the text recorded as something to put back", how)
	}
	if err := e.putItBack(context.Background(), howToUndo{Kind: undoByPuttingTextBack, Typed: plan.Drive.typed}); err != nil {
		t.Fatalf("putItBack: %v", err)
	}
	if len(screen.putBack) != 1 || screen.putBack[0] != "Editor/Message=a draft I was writing" {
		t.Errorf("put back %q, want the draft returned to the field", screen.putBack)
	}
}

func TestARunThatSentSomethingCannotBeUndoneByPuttingTextBack(t *testing.T) {
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })

	screen := &keepingScreen{held: map[string]string{"Message": ""}}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{answers: []synth.StepAnswer{
		{Step: typeIntoMessage()}, {Step: sendIt()}, {Done: true, Why: "sent"},
	}}}
	plan := &Plan{Drive: &DrivePlan{App: "Editor", Task: "say hello"}}

	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if how := undoFor(plan); how != "" {
		t.Errorf("undo = %q; a message that was sent is not unsent by clearing the box", how)
	}
}

func TestAFieldThatCannotBeReadIsNotOfferedForUndo(t *testing.T) {
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })

	screen := &keepingScreen{held: map[string]string{}}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{answers: typeTwice()}}
	plan := &Plan{Drive: &DrivePlan{App: "Editor", Task: "say hello"}}

	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if how := undoFor(plan); how != "" {
		t.Errorf("undo = %q, with nothing known about what the field held", how)
	}
}
