package engine

import (
	"context"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func TestTheWordingSaysWhetherToShowOrToDo(t *testing.T) {
	for _, in := range []string{
		"show me how to send a message on whatsapp",
		"Teach me to apply a LUT in Premiere",
		"how do I book a flight on makemytrip",
		"How to upload a video on youtube",
		"where is the search bar",
		"walk me through changing the wallpaper",
		"can you show me how to export this",
		"I want to learn how to make a playlist",
		"help me set up a new note",
		"book a flight, step by step",
		"guide me through the settings",
	} {
		if !asksToBeShown(in) {
			t.Errorf("%q was not read as asking to be shown", in)
		}
	}
	for _, in := range []string{
		`send "Good Morning" to Joe on whatsapp`,
		"Send this to Suhani",
		"book a flight from Delhi to Goa on makemytrip",
		"put a video on youtube",
		"type hello in TextEdit",
		"do this for me",
		"search for flights",
		"post a tweet saying how are you",
		"message mom that the show starts at eight",
		"open the howard account",
	} {
		if asksToBeShown(in) {
			t.Errorf("%q was read as asking to be shown", in)
		}
	}
}

func TestTheModeOverridesTheWordingWhenOneIsGiven(t *testing.T) {
	e := &Engine{}
	for _, c := range []struct {
		in   Ask
		kind route.Kind
		want bool
	}{
		{Ask{Instruction: "send hi to Joe"}, route.KindAppAction, false},
		{Ask{Instruction: "show me how to send hi to Joe"}, route.KindAppAction, true},
		{Ask{Instruction: "the wallpaper setting"}, route.KindAppGuide, true},
		{Ask{Instruction: "send hi to Joe", Mode: ModeFlip}, route.KindAppAction, true},
		{Ask{Instruction: "how do I send hi to Joe", Mode: ModeFlip}, route.KindAppGuide, false},
		{Ask{Instruction: "how do I send hi", Mode: ModeDo}, route.KindAppGuide, false},
		{Ask{Instruction: "send hi", Mode: ModeShow}, route.KindPageAction, true},
	} {
		if got := e.wantsShowing(c.in, c.kind); got != c.want {
			t.Errorf("%q as %s with mode %q: show=%v, want %v", c.in.Instruction, c.kind, c.in.Mode, got, c.want)
		}
	}
}

func TestAQuestionAskedToBeDoneIsDone(t *testing.T) {
	e := &Engine{UIScreen: &movingScreen{}, Steps: &scriptedSteps{}}
	res := route.Result{Shape: "how do i say hello", Route: route.Route{Kind: route.KindAppGuide, Slots: map[string]string{"intent": "say hello"}}}

	plan, err := e.doInsteadOfShowing(context.Background(), Ask{Instruction: "how do I say hello", Mode: ModeFlip}, res)
	if err != nil {
		t.Fatalf("doInsteadOfShowing: %v", err)
	}
	if plan.Drive == nil || plan.Guide != nil || plan.Drive.Task != "how do I say hello" {
		t.Fatalf("plan = %+v, want a run that does it in the window in front", plan)
	}
}

func offering(e *Engine) chan bool {
	reply := make(chan bool, 1)
	e.Reply = reply
	return reply
}

func TestTheCoachOffersToTypeAndDoesItWhenToldYes(t *testing.T) {
	quickGuide(t)
	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}
	reply := offering(e)

	var shown []GuideStep
	_, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
		if s.Offer {
			go func() {
				time.Sleep(5 * time.Millisecond)
				reply <- true
			}()
		}
	}, nil)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) != 1 || !shown[0].Offer {
		t.Fatalf("shown = %+v, want the typing step shown once with the offer on it", shown)
	}
	if screen.acts != 1 {
		t.Errorf("acted %d time(s), want the one step typed for them", screen.acts)
	}
}

func TestSayingNoLeavesTheStepToThePerson(t *testing.T) {
	quickGuide(t)
	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}
	reply := offering(e)

	var shown []GuideStep
	_, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
		if s.Offer {
			go func() {
				time.Sleep(5 * time.Millisecond)
				reply <- false
			}()
			return
		}
		screen.changesAfter = screen.captures + 2
	}, nil)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) != 2 || !shown[0].Offer || shown[1].Offer {
		t.Fatalf("shown = %+v, want the offer, then the same step without it", shown)
	}
	if shown[0].Number != shown[1].Number {
		t.Errorf("the step number moved from %d to %d on a no", shown[0].Number, shown[1].Number)
	}
	if screen.acts != 0 {
		t.Errorf("acted %d time(s) after being told no", screen.acts)
	}
}

func TestOnlyStepsThatEnterAValueAreOffered(t *testing.T) {
	field := uiact.Element{Role: "AXTextField", Title: "From", Enabled: true}
	button := uiact.Element{Role: "AXButton", Title: "Search flights", Enabled: true}
	send := uiact.Element{Role: "AXButton", Title: "Send", Enabled: true}
	date := uiact.Element{Role: "AXButton", Title: "14 October", Enabled: true}

	if !uiact.EntersAValue(uiact.Step{Do: "type", Text: "Delhi"}, field) {
		t.Error("typing a city was not offered")
	}
	if !uiact.EntersAValue(uiact.Step{Do: "click", Fill: true}, date) {
		t.Error("picking the date the model marked as a value was not offered")
	}
	if uiact.EntersAValue(uiact.Step{Do: "click"}, button) {
		t.Error("an ordinary click was offered")
	}
	if uiact.EntersAValue(uiact.Step{Do: "click", Fill: true}, send) {
		t.Error("the click that sends was offered; that one is theirs")
	}
}

func TestWithNobodyToAnswerNothingIsOffered(t *testing.T) {
	quickGuide(t)
	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 2}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}

	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) { shown = append(shown, s) }, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) != 1 || shown[0].Offer {
		t.Errorf("shown = %+v, want the step with no offer", shown)
	}
}
