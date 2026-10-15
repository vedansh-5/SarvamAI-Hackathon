package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type rulebook store.Rules

func (r rulebook) Rules(context.Context) (store.Rules, error) { return store.Rules(r), nil }

func TestARunIsRefusedInAPlaceTheRulesForbid(t *testing.T) {
	screen := &movingScreen{}
	e := &Engine{
		UIScreen: screen,
		Rulebook: rulebook{Never: []string{"editor", "mybank.com"}},
		Steps:    &scriptedSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}}},
	}

	_, err := e.Commit(context.Background(), drivePlan())
	var no *NotAllowedError
	if !errors.As(err, &no) || no.Where != "Editor" {
		t.Fatalf("err = %v, want to be refused in Editor", err)
	}
	if screen.acts != 0 {
		t.Errorf("acted %d time(s) in a place the rules forbid", screen.acts)
	}
}

func TestTheRulesNameSitesAsWellAsApplications(t *testing.T) {
	r := store.Rules{Never: []string{"mybank.com", "Mail"}}
	site := store.Rules{Never: []string{"makemytrip"}}
	if _, no := forbidden(site, "makemytrip.com"); !no {
		t.Error("a site named without its ending was allowed")
	}
	if where, no := forbidden(site, "notmakemytrip.com"); no {
		t.Errorf("%q was refused for containing the name", where)
	}
	for _, place := range []string{"mybank.com", "netbanking.mybank.com", "mail", "\u200eMail"} {
		if _, no := forbidden(r, place); !no {
			t.Errorf("%q was allowed", place)
		}
	}
	for _, place := range []string{"notmybank.org", "Gmail Notifier", "", "Notes"} {
		if where, no := forbidden(r, place); no {
			t.Errorf("%q was refused as %q", place, where)
		}
	}
}

func payScreen(amount string) uiact.Tree {
	t := editorTree()
	t.Elements = append(t.Elements,
		uiact.Element{Role: "AXStaticText", Title: "Total " + amount, Enabled: true, Position: []int{10, 300}, Size: []int{100, 20}, Path: []int{0, 5}},
		uiact.Element{Role: "AXButton", Title: "Pay now", Enabled: true, Position: []int{10, 340}, Size: []int{100, 30}, Path: []int{0, 6}},
	)
	return t
}

func payNow() uiact.Step {
	return uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Pay now"}, Describe: "Click Pay now"}
}

func TestAStepThatPaysOverTheLimitIsNotTaken(t *testing.T) {
	e := &Engine{Rulebook: rulebook{SpendLimit: 5000}}
	ctx := context.Background()

	err := e.overTheLimit(ctx, payScreen("\u20B97,499.00"), payNow())
	var no *NotAllowedError
	if !errors.As(err, &no) || !strings.Contains(no.Rule, "7499") || !strings.Contains(no.Rule, "5000") {
		t.Fatalf("err = %v, want the step refused with both numbers said", err)
	}
	if err := e.overTheLimit(ctx, payScreen("\u20B94,200"), payNow()); err != nil {
		t.Errorf("a payment under the limit was refused: %v", err)
	}
	if err := e.overTheLimit(ctx, payScreen("$9,999"), typeIntoMessage()); err != nil {
		t.Errorf("typing was refused for an amount on the screen: %v", err)
	}
	if err := (&Engine{}).overTheLimit(ctx, payScreen("\u20B999,999"), payNow()); err != nil {
		t.Errorf("a payment was refused with no limit set: %v", err)
	}
}

func TestAmountsAreReadInTheWaysTheyAreWritten(t *testing.T) {
	tree := uiact.Tree{Elements: []uiact.Element{
		{Role: "AXStaticText", Title: "Fare \u20B9 4,250"},
		{Role: "AXStaticText", Value: "Rs. 1,200.50 taxes"},
		{Role: "AXStaticText", Title: "$19.99 / month"},
		{Role: "AXStaticText", Title: "Flight 6E 2043 at 07:10"},
	}}
	got := uiact.Amounts(tree)
	if len(got) != 3 || got[0] != 4250 || got[1] != 1200.50 || got[2] != 19.99 {
		t.Errorf("amounts = %v, want the three prices and not the flight number", got)
	}
}

func TestCarryingOnIsAllowedThroughTheSameGateThatRefusesASecondCommit(t *testing.T) {
	quickLook(t, 0)
	screen := &breaksOnce{failAt: 2}
	e := &Engine{UIScreen: screen, Steps: &recordingSteps{answers: []synth.StepAnswer{
		{Step: typeIntoMessage()}, {Step: pressEnter()}, {Step: pressEnter()}, {Done: true},
	}}}

	plan := drivePlan()
	if _, err := e.Commit(context.Background(), plan); err == nil {
		t.Fatal("the run was meant to break")
	}
	if _, err := e.Commit(context.Background(), plan); err != nil {
		t.Fatalf("carrying on was refused: %v", err)
	}
	if _, err := e.Commit(context.Background(), plan); err == nil {
		t.Fatal("a finished plan was run a third time")
	}
}

func TestInAPlaceMarkedAskFirstEveryStepWaits(t *testing.T) {
	quickLook(t, 0)
	var paused []GuideStep
	resume := make(chan struct{}, 1)
	e := &Engine{
		UIScreen: &movingScreen{},
		Rulebook: rulebook{AskFirst: []string{"editor"}},
		Steps: &scriptedSteps{answers: []synth.StepAnswer{
			{Step: clickMessage()}, {Step: pressEnter()}, {Done: true},
		}},
	}
	e.ShowStep = func(GuideStep) {}
	e.Resume = resume
	e.Paused = func(g GuideStep) {
		paused = append(paused, g)
		go func() { resume <- struct{}{} }()
	}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(paused) != 2 || !strings.Contains(paused[0].Ask, "ask before every step") {
		t.Fatalf("asked %d time(s): %+v; want before each of the two steps", len(paused), paused)
	}
}

func TestElsewhereOrdinaryStepsDoNotWait(t *testing.T) {
	quickLook(t, 0)
	var paused []GuideStep
	e := &Engine{
		UIScreen: &movingScreen{},
		Rulebook: rulebook{AskFirst: []string{"Mail"}},
		Steps:    &scriptedSteps{answers: []synth.StepAnswer{{Step: clickMessage()}, {Done: true}}},
	}
	e.ShowStep = func(GuideStep) {}
	e.Resume = make(chan struct{}, 1)
	e.Paused = func(g GuideStep) { paused = append(paused, g) }

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(paused) != 0 {
		t.Errorf("asked %d time(s) in an application the rule does not name", len(paused))
	}
}
