package uiact

import (
	"strings"
	"testing"
)

// The System Settings sidebar, which is what found this: every item is a row,
// and the model keeps asking for a button.
func settingsTree() Tree {
	return Tree{
		App:    "System Settings",
		Window: "Wallpaper",
		Elements: []Element{
			{Role: "AXRow", Title: "Wallpaper", Enabled: true,
				Position: []int{20, 100}, Size: []int{180, 28}, Path: []int{0, 1}},
			{Role: "AXRow", Title: "Displays", Enabled: true,
				Position: []int{20, 130}, Size: []int{180, 28}, Path: []int{0, 2}},
			{Role: "AXButton", Title: "Add Folder", Enabled: true,
				Position: []int{300, 400}, Size: []int{90, 24}, Path: []int{0, 3}},
		},
	}
}

func TestRepairFixesTheRoleTheWindowDisagreesWith(t *testing.T) {
	step := Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Wallpaper"}}

	fixed, what, ok := Repair(step, settingsTree())
	if !ok {
		t.Fatal("Repair did nothing, though the name is on screen as a row")
	}
	if fixed.Target.Role != "AXRow" {
		t.Errorf("role = %q, want AXRow", fixed.Target.Role)
	}
	if fixed.Target.Title != "Wallpaper" {
		t.Errorf("title = %q, want it left alone", fixed.Target.Title)
	}
	if !strings.Contains(what, "AXRow") {
		t.Errorf("what = %q, want it to say what the control actually is", what)
	}
}

// A repaired step has to pass the same check as one the model wrote. If it does
// not, the repair is worthless — this is the assertion that makes the fix a fix.
func TestARepairedStepValidates(t *testing.T) {
	tree := settingsTree()
	step := Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Wallpaper"}}

	if Validate(Program{App: tree.App, Steps: []Step{step}}, tree) == nil {
		t.Fatal("the unrepaired step already validates, so this proves nothing")
	}

	fixed, _, ok := Repair(step, tree)
	if !ok {
		t.Fatal("Repair did nothing")
	}
	if err := Validate(Program{App: tree.App, Steps: []Step{fixed}}, tree); err != nil {
		t.Errorf("the repaired step still does not validate: %v", err)
	}
}

// Nothing to repair when the step is already right. Repair must not churn a step
// that works.
func TestRepairLeavesAWorkingStepAlone(t *testing.T) {
	step := Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Add Folder"}}

	if _, _, ok := Repair(step, settingsTree()); ok {
		t.Error("Repair changed a step that already fits the window")
	}
}

// Two controls share the name, so which one was meant is a question rather than a
// mistake. Guessing it is how something gets clicked that nobody asked for.
func TestRepairWillNotGuessBetweenTwoControls(t *testing.T) {
	tree := settingsTree()
	tree.Elements = append(tree.Elements, Element{
		Role: "AXStaticText", Title: "Wallpaper", Enabled: true,
		Position: []int{400, 60}, Size: []int{100, 20}, Path: []int{0, 4},
	})

	step := Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Wallpaper"}}
	if _, _, ok := Repair(step, tree); ok {
		t.Error("Repair picked one of two controls with the same name")
	}
}

// A name that is nowhere on screen is the model being wrong about the window, not
// about what to call something. There is nothing to correct it to.
func TestRepairCannotInventAControl(t *testing.T) {
	step := Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Nothing Like This"}}

	if _, _, ok := Repair(step, settingsTree()); ok {
		t.Error("Repair invented a target for a name that is not on screen")
	}
}

// The role was right and something else was wrong — disabled, not pressable. That
// is not a naming mistake and renaming it would not help.
func TestRepairDoesNotTouchADisabledControl(t *testing.T) {
	tree := settingsTree()
	tree.Elements[2].Enabled = false

	step := Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Add Folder"}}
	if _, _, ok := Repair(step, tree); ok {
		t.Error("Repair rewrote a step whose control was merely disabled")
	}
}

// An untitled target gives nothing to match on.
func TestRepairNeedsANameToWorkFrom(t *testing.T) {
	step := Step{Do: "click", Target: Selector{Role: "AXCheckBox"}}

	if _, _, ok := Repair(step, settingsTree()); ok {
		t.Error("Repair changed a step with no name to match")
	}
}

// What is left for the model to answer is the ambiguous case, so the refusal has
// to say how many of each there are and that an index is wanted.
func TestTheRefusalNamesTheOptionsAndTheirCounts(t *testing.T) {
	tree := settingsTree()
	tree.Elements[1].Title = "Wallpaper"

	step := Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Wallpaper"}}
	err := Validate(Program{App: tree.App, Steps: []Step{step}}, tree)
	if err == nil {
		t.Fatal("expected a refusal")
	}

	why := err.Error()
	if !strings.Contains(why, "2 of them") {
		t.Errorf("refusal does not say how many there are: %q", why)
	}
	if !strings.Contains(why, "index") {
		t.Errorf("refusal does not ask for an index: %q", why)
	}
}

func TestTheRefusalTellsTheModelWhatToWriteInstead(t *testing.T) {
	step := Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Wallpaper"}}
	err := Validate(Program{App: "System Settings", Steps: []Step{step}}, settingsTree())
	if err == nil {
		t.Fatal("expected a refusal")
	}

	if why := err.Error(); !strings.Contains(why, "name that role instead") {
		t.Errorf("refusal describes the problem without saying what to do: %q", why)
	}
}
