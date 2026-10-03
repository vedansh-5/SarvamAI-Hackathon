package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type readableScreen struct {
	fakeScreen
	shown uiact.ScreenText
	reads int
}

func (r *readableScreen) ReadText(context.Context, string) (uiact.ScreenText, error) {
	r.reads++
	return r.shown, nil
}

type screenReader struct {
	answer synth.ReadAnswer
	asked  []synth.ReadRequest
}

func (s *screenReader) ReadScreen(_ context.Context, req synth.ReadRequest) (synth.ReadAnswer, error) {
	s.asked = append(s.asked, req)
	return s.answer, nil
}

var chat = &screen.Context{AppName: "Chat", WindowTitle: "Joe"}

func reading(answer synth.ReadAnswer, lines ...string) (*Engine, *readableScreen, *screenReader) {
	shown := &readableScreen{fakeScreen: fakeScreen{front: "Chat"}, shown: uiact.ScreenText{App: "Chat", Window: "Joe", Lines: lines}}
	model := &screenReader{answer: answer}
	return &Engine{UIScreen: shown, Reader: model, Log: func(string, ...any) {}}, shown, model
}

func TestAQuestionAboutTheScreenIsAnsweredFromItsText(t *testing.T) {
	e, shown, model := reading(synth.ReadAnswer{Text: "Yes, 7 pm at the usual place"}, "Joe", "Are we still on for Friday?", "Yes, 7 pm at the usual place")

	plan, err := e.readTheScreen(context.Background(), Ask{Instruction: "what did Joe say last", Context: chat})
	if err != nil {
		t.Fatalf("readTheScreen: %v", err)
	}
	if shown.reads != 1 || len(model.asked) != 1 || len(model.asked[0].Lines) != 3 {
		t.Fatalf("read %d time(s), asked %+v; want the window's three lines sent once", shown.reads, model.asked)
	}
	if plan.Copy != "Yes, 7 pm at the usual place" || plan.NeedsCommit() {
		t.Errorf("copy = %q, needs commit = %v; want the answer, and nothing to confirm", plan.Copy, plan.NeedsCommit())
	}
	if !strings.Contains(plan.Say, "was sent to the model") || !strings.Contains(plan.Say, "Chat") {
		t.Errorf("say = %q, want it to say whose text was sent", plan.Say)
	}
}

func TestAnApplicationUnderTheNeverRuleIsNotRead(t *testing.T) {
	e, shown, model := reading(synth.ReadAnswer{Text: "x"}, "private")
	e.Rulebook = rulebook{Never: []string{"Chat"}}

	_, err := e.readTheScreen(context.Background(), Ask{Instruction: "what did Joe say last", Context: chat})
	var no *NotAllowedError
	if !errors.As(err, &no) {
		t.Fatalf("err = %v, want it refused by the rule", err)
	}
	if shown.reads != 0 || len(model.asked) != 0 {
		t.Errorf("read %d time(s) and asked the model %d time(s) about a window the rules protect", shown.reads, len(model.asked))
	}
}

func TestWhatIsNotOnScreenIsSaidNotGuessed(t *testing.T) {
	e, _, _ := reading(synth.ReadAnswer{Cannot: true, Why: "The conversation shown is with Joe, not Suhani."}, "Joe", "hello")

	plan, err := e.readTheScreen(context.Background(), Ask{Instruction: "what did Suhani say", Context: chat})
	if err != nil {
		t.Fatal(err)
	}
	if plan.SayTitle != "Not on screen" || plan.Copy != "" || !strings.Contains(plan.Say, "not Suhani") {
		t.Errorf("plan = %q / %q, want it to say the answer is not on screen", plan.SayTitle, plan.Say)
	}
}

func TestAWindowWithNoTextAsksNoModel(t *testing.T) {
	e, _, model := reading(synth.ReadAnswer{Text: "x"})

	plan, err := e.readTheScreen(context.Background(), Ask{Instruction: "what does this say", Context: chat})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.asked) != 0 || plan.SayTitle != "Nothing to read" {
		t.Errorf("asked %d time(s), title %q; want no call for an empty window", len(model.asked), plan.SayTitle)
	}
}
