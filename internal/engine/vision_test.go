package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func blindWindow() uiact.Tree {
	return uiact.Tree{App: "Editor", Window: "Editor", Elements: []uiact.Element{
		{Role: "AXGroup", Enabled: true, Position: []int{0, 0}, Size: []int{900, 600}, Path: []int{0}},
	}}
}

func TestAnAppThatCannotBeReadIsCoachedByLookingAtIt(t *testing.T) {
	quickGuide(t)

	typed := editorTree()
	typed.Elements[0].Value = "hello"
	eyes := &fakeScreen{tree: editorTree(), afterwards: &typed, changesAfter: 3}
	blind := &fakeScreen{tree: blindWindow()}
	e := &Engine{UIScreen: blind, Eyes: eyes, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}

	plan, err := e.planGuideFor(context.Background(), "how do I say hello", "", nil, &screen.Context{AppName: "Editor"})
	if err != nil {
		t.Fatalf("planGuideFor: %v", err)
	}
	if !plan.Guide.Vision || !strings.Contains(plan.Guide.Preview, "looking at its window") {
		t.Fatalf("guide = %+v, want it to say it will look at the window", plan.Guide)
	}
	if eyes.captures != 0 {
		t.Errorf("looked %d time(s) before the run started; a look costs a model call", eyes.captures)
	}

	readsBefore := blind.captures
	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), plan, func(s GuideStep) { shown = append(shown, s) }, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) == 0 || shown[0].W != 80 {
		t.Fatalf("shown = %+v, want the control pointed at where it was seen", shown)
	}
	if blind.captures != readsBefore {
		t.Errorf("read the unreadable window %d more time(s) during the run", blind.captures-readsBefore)
	}
	if eyes.acts != 0 {
		t.Errorf("acted %d time(s) through a picture", eyes.acts)
	}
}

func TestAnAppThatCanBeReadIsNotLookedAt(t *testing.T) {
	readable := uiact.Tree{App: "Editor", Window: "Editor"}
	for i, name := range []string{"New", "Open", "Save", "Print", "Share", "Find"} {
		readable.Elements = append(readable.Elements, uiact.Element{
			Role: "AXButton", Title: name, Enabled: true,
			Position: []int{10 + 60*i, 10}, Size: []int{50, 24}, Path: []int{i},
		})
	}
	e := &Engine{UIScreen: &fakeScreen{tree: readable}, Eyes: &fakeScreen{tree: editorTree()}, Steps: &scriptedSteps{}}

	plan, err := e.planGuideFor(context.Background(), "how do I save", "", nil, &screen.Context{AppName: "Editor"})
	if err != nil {
		t.Fatalf("planGuideFor: %v", err)
	}
	if plan.Guide.Vision {
		t.Error("a window that reads fine was sent to be looked at, which is slower and less exact")
	}
}

func TestWithoutEyesAnUnreadableAppIsRefusedAsBefore(t *testing.T) {
	e := &Engine{UIScreen: &fakeScreen{tree: uiact.Tree{App: "Editor"}}, Steps: &scriptedSteps{}}
	if _, err := e.planGuideFor(context.Background(), "how do I save", "", nil, &screen.Context{AppName: "Editor"}); err == nil {
		t.Fatal("coached a window with nothing in it and no way to look at it")
	}
}
