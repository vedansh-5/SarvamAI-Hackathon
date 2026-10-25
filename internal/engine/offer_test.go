package engine

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type formScreen struct {
	mu     sync.Mutex
	tree   uiact.Tree
	filled []string
}

func formTree() uiact.Tree {
	return uiact.Tree{
		App:    "Editor",
		Window: "Editor",
		Elements: []uiact.Element{
			{Role: "AXTextField", Title: "Name", Enabled: true, Position: []int{100, 100}, Size: []int{200, 30}, Path: []int{0, 1}},
			{Role: "AXTextField", Title: "City", Enabled: true, Position: []int{100, 150}, Size: []int{200, 30}, Path: []int{0, 2}},
		},
	}
}

func (f *formScreen) Capture(context.Context, string) (uiact.Tree, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.tree
	now.Elements = append([]uiact.Element(nil), f.tree.Elements...)
	return now, nil
}

func (f *formScreen) put(title, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.tree.Elements {
		if f.tree.Elements[i].Title == title {
			f.tree.Elements[i].Value = text
		}
	}
}

func (f *formScreen) Act(_ context.Context, p uiact.Program, t uiact.Tree) error {
	if err := uiact.Validate(p, t); err != nil {
		return err
	}
	for _, step := range p.Steps {
		f.put(step.Target.Title, step.Text)
		f.mu.Lock()
		f.filled = append(f.filled, step.Target.Title)
		f.mu.Unlock()
	}
	return nil
}

func (f *formScreen) BringToFront(context.Context, string) error { return nil }
func (f *formScreen) Frontmost(context.Context) (string, error)  { return "Editor", nil }
func (f *formScreen) Type(context.Context, string) error         { return nil }

func (f *formScreen) ValueOf(_ context.Context, t uiact.Tree, sel uiact.Selector) (string, bool) {
	for _, el := range t.Elements {
		if el.Title == sel.Title {
			return el.Value, true
		}
	}
	return "", false
}

func (f *formScreen) PutBack(_ context.Context, _ string, sel uiact.Selector, value string) error {
	f.put(sel.Title, value)
	return nil
}

func typeInto(title, text string) uiact.Step {
	return uiact.Step{
		Do:       "type",
		Target:   uiact.Selector{Role: "AXTextField", Title: title},
		Text:     text,
		Describe: "Type " + text + " into " + title,
	}
}

func twoValues() *scriptedSteps {
	return &scriptedSteps{answers: []synth.StepAnswer{
		{Step: typeInto("Name", "Vedansh")},
		{Step: typeInto("City", "Delhi")},
		{Done: true},
	}}
}

func noPauseBeforeActing(t *testing.T) {
	t.Helper()
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })
}

func answering(reply chan OfferReply, with OfferReply) {
	go func() {
		time.Sleep(5 * time.Millisecond)
		reply <- with
	}()
}

func TestYesToAllEntersTheRestWithoutAsking(t *testing.T) {
	quickGuide(t)
	noPauseBeforeActing(t)
	screen := &formScreen{tree: formTree()}
	deeds := &recordedDeeds{}
	e := &Engine{UIScreen: screen, Deeds: deeds, Steps: twoValues()}
	reply := offering(e)

	var shown []GuideStep
	_, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
		if s.Offer {
			answering(reply, OfferTakenForAll)
		}
	}, nil)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}

	if len(shown) != 2 {
		t.Fatalf("shown = %+v, want the two steps once each", shown)
	}
	if !shown[0].Offer || !shown[0].OfferAll {
		t.Errorf("the first value was shown as %+v, want the offer with yes-to-all on it", shown[0])
	}
	if shown[1].Offer || !shown[1].Doing {
		t.Errorf("the second value was shown as %+v, want it being entered with nothing asked", shown[1])
	}
	if strings.Join(screen.filled, ",") != "Name,City" {
		t.Errorf("entered %v, want both values", screen.filled)
	}

	if len(deeds.deeds) != 1 {
		t.Fatalf("recorded %d deed(s), want the one run written down", len(deeds.deeds))
	}
	deed := deeds.deeds[0]
	if !strings.Contains(deed.Did, "Type Vedansh into Name") || !strings.Contains(deed.Did, "Type Delhi into City") {
		t.Errorf("the record says %q, want both values named", deed.Did)
	}
	if !deed.Verified {
		t.Error("both values landed and the record says they were not seen to")
	}
	if !strings.Contains(deed.Undo, undoByPuttingTextBack) {
		t.Errorf("undo = %q, want what the fields held kept so it can be put back", deed.Undo)
	}
}

func TestASingleYesIsAskedAgainAtTheNextValue(t *testing.T) {
	quickGuide(t)
	noPauseBeforeActing(t)
	screen := &formScreen{tree: formTree()}
	e := &Engine{UIScreen: screen, Steps: twoValues()}
	reply := offering(e)

	var shown []GuideStep
	_, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
		if s.Offer {
			answering(reply, OfferTaken)
		}
	}, nil)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) != 2 || !shown[0].Offer || !shown[1].Offer || shown[1].Doing {
		t.Fatalf("shown = %+v, want an offer on each value", shown)
	}
	if len(screen.filled) != 2 {
		t.Errorf("entered %v, want both, each after a yes", screen.filled)
	}
}

func TestSayingNoOnceEndsTheOffersForTheRun(t *testing.T) {
	quickGuide(t)
	noPauseBeforeActing(t)
	screen := &formScreen{tree: formTree()}
	deeds := &recordedDeeds{}
	e := &Engine{UIScreen: screen, Deeds: deeds, Steps: twoValues()}
	reply := offering(e)

	var shown []GuideStep
	_, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
		if s.Offer {
			answering(reply, OfferDeclined)
			return
		}
		title, text := "Name", "Vedansh"
		if s.Number == 2 {
			title, text = "City", "Delhi"
		}
		go func() {
			time.Sleep(5 * time.Millisecond)
			screen.put(title, text)
		}()
	}, nil)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}

	if len(shown) != 3 || !shown[0].Offer || shown[1].Offer || shown[2].Offer {
		t.Fatalf("shown = %+v, want one offer, then both steps left to the person", shown)
	}
	if shown[2].Number != 2 {
		t.Errorf("the last step shown is number %d, want the second value", shown[2].Number)
	}
	if len(screen.filled) != 0 {
		t.Errorf("entered %v after being told no", screen.filled)
	}
	if len(deeds.deeds) != 0 {
		t.Errorf("recorded %+v though VANS entered nothing", deeds.deeds)
	}
}

func TestNothingIsOfferedWhereVANSMustNeverAct(t *testing.T) {
	quickGuide(t)
	noPauseBeforeActing(t)
	screen := &formScreen{tree: formTree()}
	e := &Engine{
		UIScreen: screen,
		Rulebook: rulebook{Never: []string{"Editor"}},
		Steps:    &scriptedSteps{answers: []synth.StepAnswer{{Step: typeInto("Name", "Vedansh")}, {Done: true}}},
	}
	offering(e)

	var shown []GuideStep
	_, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
		go func() {
			time.Sleep(5 * time.Millisecond)
			screen.put("Name", "Vedansh")
		}()
	}, nil)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) != 1 || shown[0].Offer || shown[0].OfferAll {
		t.Fatalf("shown = %+v, want the step with nothing offered", shown)
	}
	if len(screen.filled) != 0 {
		t.Errorf("entered %v in a place VANS must never act", screen.filled)
	}
}

func TestWhereEveryStepIsAskedFirstThereIsNoYesToAll(t *testing.T) {
	quickGuide(t)
	noPauseBeforeActing(t)
	screen := &formScreen{tree: formTree()}
	e := &Engine{UIScreen: screen, Rulebook: rulebook{AskFirst: []string{"Editor"}}, Steps: twoValues()}
	reply := offering(e)

	var shown []GuideStep
	_, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
		if s.Offer {
			answering(reply, OfferTakenForAll)
		}
	}, nil)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) != 2 {
		t.Fatalf("shown = %+v, want the two steps", shown)
	}
	for _, s := range shown {
		if !s.Offer || s.OfferAll || s.Doing {
			t.Errorf("step %d was shown as %+v, want a plain offer each time", s.Number, s)
		}
	}
	if len(screen.filled) != 2 {
		t.Errorf("entered %v, want both, each after its own yes", screen.filled)
	}
}
