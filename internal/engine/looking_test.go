package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type selecting struct {
	movingScreen
	picked string
	asked  int
}

func (s *selecting) Selection(context.Context, string) (string, bool) {
	s.asked++
	return s.picked, s.picked != ""
}

var mail = &screen.Context{AppName: "Mail", WindowTitle: "Re: Friday plans"}

func TestWhatWasInFrontIsOnlySentWhenTheRequestPointsAtIt(t *testing.T) {
	for _, in := range []string{
		"reply to this",
		"translate that into Hindi",
		"send it to Suhani",
		"rename the selected file",
		"summarise the one I'm looking at",
		"book the flight on screen",
		"close the current tab",
		"add a heading here",
	} {
		if !refersToWhatIsInFront(in) {
			t.Errorf("%q points at what is in front and nothing was sent with it", in)
		}
	}
	for _, in := range []string{
		`send "Good Morning" to Joe on whatsapp`,
		"book a flight from Delhi to Goa on makemytrip",
		"how do I change the wallpaper",
		"search for flights",
		"open WhatsApp",
		"type hello in TextEdit",
	} {
		if refersToWhatIsInFront(in) {
			t.Errorf("%q says nothing about what is in front, and it would have been sent anyway", in)
		}
	}
}

func TestTheSelectionIsReadOnlyForARequestThatPointsAtIt(t *testing.T) {
	s := &selecting{picked: "Can we move it to Saturday?"}
	e := &Engine{UIScreen: s}

	if note := e.lookingFor(context.Background(), Ask{Instruction: "book a flight to Goa", Context: mail}); note != "" || s.asked != 0 {
		t.Fatalf("note = %q after %d read(s) of the selection; nothing in that request asks for it", note, s.asked)
	}

	note := e.lookingFor(context.Background(), Ask{Instruction: "reply to this", Context: mail})
	if !strings.Contains(note, "looking at Mail") || !strings.Contains(note, "Can we move it to Saturday?") {
		t.Errorf("note = %q, want the window and the selected text", note)
	}
	if !strings.Contains(note, "mean that text") {
		t.Errorf("note = %q, want it to say what \"this\" refers to", note)
	}
}

func TestALongSelectionIsCutShort(t *testing.T) {
	note := whatTheyWereLookingAt(mail, strings.Repeat("word ", 400))
	if len(note) > longestSelection+400 {
		t.Errorf("note is %d characters; a whole document was sent as the selection", len(note))
	}
}

func TestWhatWasInFrontGoesBesideTheTaskNotIntoIt(t *testing.T) {
	steps := &recordingSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}}}
	e := &Engine{UIScreen: &movingScreen{}, Steps: steps}

	plan := drivePlan()
	plan.Drive.Looking = whatTheyWereLookingAt(mail, "")
	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	task := steps.requests[0].Task
	if !strings.Contains(task, "say hello") || !strings.Contains(task, `looking at Mail, in the window "Re: Friday plans"`) {
		t.Errorf("the model was given %q", task)
	}
	if plan.Drive.Task != "say hello" {
		t.Errorf("the task itself became %q", plan.Drive.Task)
	}
}

func TestNothingIsSaidAboutTheScreenWhenItIsNotKnown(t *testing.T) {
	for _, at := range []*screen.Context{nil, {}, {AppName: "vans"}} {
		if note := whatTheyWereLookingAt(at, "text"); note != "" {
			t.Errorf("said %q about a screen nobody read", note)
		}
	}
}

var _ uiact.SelectionReader = (*selecting)(nil)
