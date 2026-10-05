package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

type textWorker struct {
	answer synth.TextAnswer
	asked  []synth.TextRequest
}

func (w *textWorker) WorkOnText(_ context.Context, req synth.TextRequest) (synth.TextAnswer, error) {
	w.asked = append(w.asked, req)
	return w.answer, nil
}

func textEngine(t *testing.T, kind route.Kind, picked string, w *textWorker) *Engine {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	return &Engine{
		UIScreen: &selecting{picked: picked},
		Texts:    w,
		Router: &route.Router{
			Decider: &fixedDecider{d: route.Decision{Kind: kind, Slots: map[string]string{"intent": "translate it to Hindi"}}},
			Cache:   st,
		},
		Log: func(string, ...any) {},
	}
}

func TestSelectedTextIsWorkedOnAndGivenBackToCopy(t *testing.T) {
	w := &textWorker{answer: synth.TextAnswer{Text: "शनिवार को चलें?"}}
	e := textEngine(t, route.KindText, "Can we move it to Saturday?", w)

	plan, err := e.Ask(context.Background(), Ask{Instruction: "translate this to Hindi", Context: mail})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if plan.Say != "शनिवार को चलें?" || plan.Copy != plan.Say {
		t.Errorf("say = %q, copy = %q, want the translation in both", plan.Say, plan.Copy)
	}
	if plan.NeedsCommit() {
		t.Error("working on text touches nothing, so there is nothing to confirm")
	}
	if len(w.asked) != 1 || w.asked[0].Text != "Can we move it to Saturday?" {
		t.Errorf("the model was given %+v, want the selected text", w.asked)
	}
}

func TestPointingAtTextWithNothingSelectedAsksForASelection(t *testing.T) {
	w := &textWorker{answer: synth.TextAnswer{Text: "should not be reached"}}
	e := textEngine(t, route.KindText, "", w)

	plan, err := e.Ask(context.Background(), Ask{Instruction: "translate this to Hindi", Context: mail})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(w.asked) != 0 {
		t.Errorf("asked the model %d time(s) with no text to give it", len(w.asked))
	}
	if !strings.Contains(plan.Say, "Select the text") || !strings.Contains(plan.Say, "Mail") {
		t.Errorf("say = %q, want it to ask for a selection in Mail", plan.Say)
	}
	if plan.Copy != "" {
		t.Errorf("copy = %q, want nothing to copy", plan.Copy)
	}
}

func TestTextWrittenInTheRequestNeedsNoSelection(t *testing.T) {
	w := &textWorker{answer: synth.TextAnswer{Text: "buenos días"}}
	e := textEngine(t, route.KindText, "", w)

	plan, err := e.Ask(context.Background(), Ask{Instruction: `translate "good morning" to Spanish`, Context: mail})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(w.asked) != 1 || plan.Say != "buenos días" {
		t.Errorf("say = %q after %d ask(s), want the translation from one", plan.Say, len(w.asked))
	}
}

func TestQuotedTextIsHandedOverRatherThanLeftToBeNoticed(t *testing.T) {
	w := &textWorker{answer: synth.TextAnswer{Text: "buenos días"}}
	e := textEngine(t, route.KindText, "", w)

	if _, err := e.Ask(context.Background(), Ask{Instruction: `translate "good morning" to Spanish`, Context: mail}); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(w.asked) != 1 || w.asked[0].Text != "good morning" {
		t.Errorf("the model was given %+v, want the quoted text as the text to work on", w.asked)
	}
}

func TestASelectionIsUsedEvenWhenTheRequestQuotesSomething(t *testing.T) {
	w := &textWorker{answer: synth.TextAnswer{Text: "x"}}
	e := textEngine(t, route.KindText, "Can we move it to Saturday?", w)

	if _, err := e.Ask(context.Background(), Ask{Instruction: `reply to this saying "Saturday works"`, Context: mail}); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(w.asked) != 1 || w.asked[0].Text != "Can we move it to Saturday?" {
		t.Errorf("the model was given %+v, want the selected text", w.asked)
	}
}

func TestTheTextInARequestIsFound(t *testing.T) {
	for asked, want := range map[string]string{
		`translate "good morning" to Spanish`:                           "good morning",
		"translate \u201Cgood morning, see you at three\u201D to Hindi": "good morning, see you at three",
		`make "hi" formal, like "Dear Sir, good day to you"`:            "Dear Sir, good day to you",
		"summarise: the meeting moved to 3:30 on Friday":                "the meeting moved to 3:30 on Friday",
		"translate this to Hindi":                                       "",
		"reply saying 3:30 works":                                       "",
		`translate "" to Hindi`:                                         "",
	} {
		if got := textInTheRequest(asked); got != want {
			t.Errorf("textInTheRequest(%q) = %q, want %q", asked, got, want)
		}
	}
}

func TestASelectionBeatsThePageWhenTheRequestPointsAtIt(t *testing.T) {
	w := &textWorker{answer: synth.TextAnswer{Text: "They want to move the meeting to Saturday."}}
	e := textEngine(t, route.KindPageHere, "Can we move it to Saturday?", w)

	plan, err := e.Ask(context.Background(), Ask{Instruction: "summarise this", Context: mail})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(w.asked) != 1 || plan.Route == nil || plan.Route.Kind != route.KindText {
		t.Errorf("asked %d time(s), route = %+v; want the selection worked on", len(w.asked), plan.Route)
	}
}

func TestTextThatCannotBeWorkedOnSaysWhy(t *testing.T) {
	w := &textWorker{answer: synth.TextAnswer{Cannot: true, Why: "That is a picture, not text."}}
	e := textEngine(t, route.KindText, "something", w)

	plan, err := e.Ask(context.Background(), Ask{Instruction: "translate this to Hindi", Context: mail})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if plan.Say != "That is a picture, not text." || plan.Copy != "" {
		t.Errorf("say = %q, copy = %q, want the reason and nothing to copy", plan.Say, plan.Copy)
	}
}
