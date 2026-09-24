package uiact

import (
	"fmt"
	"testing"
)

func rows(n int) []Element {
	out := []Element{{
		Role: "AXStaticText", Title: "", Value: "Wallpaper",
		Position: []int{10, 10}, Size: []int{100, 20}, Path: []int{0, 0},
	}}
	// Distinct names, the way wallpaper thumbnails arrive: elements are told
	// apart by role and title, so a row of identical untitled images would count
	// as one thing however many of them landed.
	for i := 0; i < n; i++ {
		out = append(out, Element{
			Role: "AXImage", Title: fmt.Sprintf("thumbnail %d", i),
			Position: []int{i * 10, 200}, Size: []int{40, 40}, Path: []int{0, i + 1},
		})
	}
	return out
}

func clickWallpaper() Step {
	return Step{Do: "click", Target: Selector{Role: "AXStaticText", Title: "Wallpaper"}}
}

// The bug: System Settings' Wallpaper pane loads thumbnails for seconds, the
// coach counted them as somebody acting, and walked through steps nobody had
// done.
func TestAWindowStillDrawingIsNotSomebodyActing(t *testing.T) {
	before := Tree{App: "System Settings", Elements: rows(2)}
	after := Tree{App: "System Settings", Elements: rows(9)}

	if PersonActedOn(before, after, clickWallpaper()) {
		t.Error("seven controls arriving on their own was read as somebody clicking")
	}
}

// The drive loop performed the step itself, so the same change is evidence there.
func TestTheDriveLoopStillTakesArrivalsAsEvidence(t *testing.T) {
	before := Tree{App: "System Settings", Elements: rows(2)}
	after := Tree{App: "System Settings", Elements: rows(9)}

	if !ActedOn(before, after, clickWallpaper()) {
		t.Error("VANS acted and the window changed, which is what it has to go on")
	}
}

func TestTheControlItselfChangingCounts(t *testing.T) {
	before := Tree{App: "System Settings", Elements: rows(2)}

	gone := Tree{App: "System Settings", Elements: rows(2)[1:]}
	if !PersonActedOn(before, gone, clickWallpaper()) {
		t.Error("the control the step pointed at disappeared, which is it being used")
	}

	focused := Tree{App: "System Settings", Elements: rows(2)}
	focused.Elements[0].Focused = true
	if !PersonActedOn(before, focused, clickWallpaper()) {
		t.Error("the control took focus, which is it being used")
	}
}

// Nothing renames a window by itself.
func TestAWindowRetitlingCounts(t *testing.T) {
	before := Tree{App: "System Settings", Window: "System Settings", Elements: rows(2)}
	after := Tree{App: "System Settings", Window: "Wallpaper", Elements: rows(2)}

	if !PersonActedOn(before, after, clickWallpaper()) {
		t.Error("the window retitled, which is navigation")
	}
}

// Typing is done when the text is there (D-093), on both paths.
func TestTypingIsJudgedByTheTextBeingThere(t *testing.T) {
	step := Step{
		Do:     "type",
		Target: Selector{Role: "AXTextField", Title: "Search"},
		Text:   "wallpaper",
	}
	before := Tree{App: "System Settings", Elements: []Element{
		{Role: "AXTextField", Title: "Search", Path: []int{0, 0}},
	}}

	half := Tree{App: "System Settings", Elements: []Element{
		{Role: "AXTextField", Title: "Search", Value: "wall", Path: []int{0, 0}},
	}}
	if PersonActedOn(before, half, step) {
		t.Error("half-typed text was taken as the step being done")
	}

	whole := Tree{App: "System Settings", Elements: []Element{
		{Role: "AXTextField", Title: "Search", Value: "wallpaper", Path: []int{0, 0}},
	}}
	if !PersonActedOn(before, whole, step) {
		t.Error("the text is there, so the step is done")
	}
}

// A key press has no click to observe and need not touch the control, so it
// keeps the looser test.
func TestAKeyPressStillGoesOnTheWindowChanging(t *testing.T) {
	step := Step{
		Do:     "press",
		Target: Selector{Role: "AXTextField", Title: "Search"},
		Key:    "Enter",
	}
	before := Tree{App: "System Settings", Elements: []Element{
		{Role: "AXTextField", Title: "Search", Path: []int{0, 0}},
	}}
	after := Tree{App: "System Settings", Elements: []Element{
		{Role: "AXTextField", Title: "Search", Path: []int{0, 0}},
		{Role: "AXStaticText", Value: "Result one", Path: []int{0, 1}},
		{Role: "AXStaticText", Value: "Result two", Path: []int{0, 2}},
	}}

	if !PersonActedOn(before, after, step) {
		t.Error("the window filled with results after Enter, which is the only signal there is")
	}
}
