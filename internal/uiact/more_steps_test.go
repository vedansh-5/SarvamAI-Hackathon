package uiact

import (
	"context"
	"strings"
	"testing"
	"time"
)

func studio() Tree {
	return Tree{App: "Studio", Elements: []Element{
		{Role: "AXButton", Title: "Clip", Enabled: true, Position: []int{100, 100}, Size: []int{80, 40}, Path: []int{0}},
		{Role: "AXGroup", Title: "Timeline", Enabled: true, Position: []int{100, 300}, Size: []int{600, 100}, Path: []int{1}},
		{Role: "AXSlider", Title: "Exposure", Value: "0.2", Enabled: true, Ranged: true, Low: -5, High: 5,
			Position: []int{100, 500}, Size: []int{200, 20}, Path: []int{2}},
		{Role: "AXButton", Title: "Publish", Enabled: false, Position: []int{400, 500}, Size: []int{80, 30}, Path: []int{3}},
	}}
}

func clip() Selector     { return Selector{Role: "AXButton", Title: "Clip"} }
func timeline() Selector { return Selector{Role: "AXGroup", Title: "Timeline"} }
func exposure() Selector { return Selector{Role: "AXSlider", Title: "Exposure"} }

func one(step Step) Program { return Program{App: "Studio", Steps: []Step{step}} }

func TestTheNewKindsOfStepAreAcceptedWhenWellFormed(t *testing.T) {
	to := timeline()
	for name, step := range map[string]Step{
		"double-click": {Do: "doubleclick", Target: clip()},
		"right-click":  {Do: "rightclick", Target: clip()},
		"hover":        {Do: "hover", Target: clip()},
		"scroll":       {Do: "scroll", Target: timeline(), Text: "down"},
		"drag":         {Do: "drag", Target: clip(), To: &to},
		"set":          {Do: "set", Target: exposure(), Text: "1.5"},
		"wait":         {Do: "wait", Target: Selector{Role: "AXButton", Title: "Publish"}, Text: "ready"},
		"wait for new": {Do: "wait", Target: Selector{Role: "AXStaticText", Title: "Upload complete"}},
	} {
		if err := Validate(one(step), studio()); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}
}

func TestTheNewKindsOfStepAreRefusedWhenTheyCannotWork(t *testing.T) {
	gone := Selector{Role: "AXGroup", Title: "Bin"}
	for name, tc := range map[string]struct {
		step Step
		want string
	}{
		"scroll nowhere":       {Step{Do: "scroll", Target: timeline(), Text: "around"}, "up, down, left or right"},
		"drag to nothing":      {Step{Do: "drag", Target: clip()}, "without saying where to"},
		"drag somewhere gone":  {Step{Do: "drag", Target: clip(), To: &gone}, "where it is dragged to"},
		"set a button":         {Step{Do: "set", Target: clip(), Text: "3"}, "not a slider or stepper"},
		"set out of range":     {Step{Do: "set", Target: exposure(), Text: "9"}, "goes from -5 to 5"},
		"set to a word":        {Step{Do: "set", Target: exposure(), Text: "brighter"}, "not a number"},
		"wait for nothing":     {Step{Do: "wait"}, "without naming what for"},
		"wait for a feeling":   {Step{Do: "wait", Target: clip(), Text: "soon"}, "appears, is ready, or goes"},
		"double-click the off": {Step{Do: "doubleclick", Target: Selector{Role: "AXButton", Title: "Publish"}}, "disabled"},
	} {
		err := Validate(one(tc.step), studio())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it refused for %q", name, err, tc.want)
		}
	}
}

func TestEachNewStepBecomesTheRightMove(t *testing.T) {
	to := timeline()
	acts, err := Plan(Program{App: "Studio", Steps: []Step{
		{Do: "doubleclick", Target: clip()},
		{Do: "drag", Target: clip(), To: &to},
		{Do: "scroll", Target: timeline(), Text: "down"},
		{Do: "set", Target: exposure(), Text: "1.5"},
		{Do: "wait", Target: clip()},
	}}, studio())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(acts) != 4 {
		t.Fatalf("acts = %+v, want four: waiting moves nothing", acts)
	}
	if acts[0].Do != "doubleclick" || acts[0].X != 140 || acts[0].Y != 120 {
		t.Errorf("double-click = %+v, want the middle of Clip", acts[0])
	}
	if acts[1].Do != "drag" || acts[1].ToX != 400 || acts[1].ToY != 350 {
		t.Errorf("drag = %+v, want it to end in the middle of the Timeline", acts[1])
	}
	if down, right := scrollBy(acts[2].Text); down <= 0 || right != 0 {
		t.Errorf("scroll down moves %d, %d", down, right)
	}
	if acts[3].Do != "set" || acts[3].Amount != 1.5 || !acts[3].Ranged {
		t.Errorf("set = %+v, want 1.5 with the slider's range", acts[3])
	}
}

type dial struct {
	value    float64
	settable bool
	step     float64
	nudges   int
}

func watchTheDial(t *testing.T, d *dial) {
	t.Helper()
	read, set, perform, pauseFor := numberReader, numberSetter, performer, nudgePause
	numberReader = func(int, []int) (float64, bool) { return d.value, true }
	numberSetter = func(_ int, _ []int, v float64) error {
		if d.settable {
			d.value = v
		}
		return nil
	}
	performer = func(_ int, _ []int, action string) (bool, error) {
		if d.step == 0 {
			return false, nil
		}
		d.nudges++
		if action == "AXIncrement" {
			d.value += d.step
		} else {
			d.value -= d.step
		}
		return true, nil
	}
	nudgePause = time.Microsecond
	t.Cleanup(func() { numberReader, numberSetter, performer, nudgePause = read, set, perform, pauseFor })
}

func TestASliderIsSetDirectlyWhenItAllowsIt(t *testing.T) {
	d := &dial{value: 0.2, settable: true, step: 0.5}
	watchTheDial(t, d)

	if err := adjust(context.Background(), 1, Act{Amount: 1.5, Ranged: true, Low: -5, High: 5}); err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if d.value != 1.5 || d.nudges != 0 {
		t.Errorf("value = %v after %d nudge(s), want it set in one go", d.value, d.nudges)
	}
}

func TestASliderThatWillNotBeSetIsSteppedThere(t *testing.T) {
	d := &dial{value: 0, settable: false, step: 0.5}
	watchTheDial(t, d)

	if err := adjust(context.Background(), 1, Act{Amount: 2, Ranged: true, Low: -5, High: 5}); err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if d.value != 2 || d.nudges != 4 {
		t.Errorf("value = %v after %d nudge(s), want 2 after 4", d.value, d.nudges)
	}

	down := &dial{value: 3, settable: false, step: 1}
	watchTheDial(t, down)
	if err := adjust(context.Background(), 1, Act{Amount: 1, Ranged: true, Low: 0, High: 10}); err != nil || down.value != 1 {
		t.Errorf("value = %v, err = %v; want it stepped down to 1", down.value, err)
	}
}

func TestASliderThatCanBeNeitherSetNorSteppedSaysSo(t *testing.T) {
	d := &dial{value: 0, settable: false, step: 0}
	watchTheDial(t, d)

	if err := adjust(context.Background(), 1, Act{Amount: 2, Ranged: true, Low: -5, High: 5}); err == nil {
		t.Error("a slider that did not move was reported as set")
	}
}

func TestASliderIsShownWithItsValueAndRange(t *testing.T) {
	text := studio().TextWithin(4000)
	if !strings.Contains(text, `AXSlider "Exposure" value=0.2 (from -5 to 5)`) {
		t.Errorf("the slider's value and range are missing:\n%s", text)
	}
}
