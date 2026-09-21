package uiact

import (
	"strings"
	"testing"
)

func treeOf(els ...Element) Tree {
	return Tree{App: "Editor", Window: "Editor", Elements: els}
}

func textArea(title, value string) Element {
	return Element{Role: "AXTextArea", Title: title, Value: value, Enabled: true, Path: []int{0}}
}

func TestChangesReportsTextThatArrived(t *testing.T) {
	before := treeOf(textArea("Message", ""))
	after := treeOf(textArea("Message", "on my way"))

	got := Changes(before, after)
	if len(got) != 1 || !strings.Contains(got[0], "on my way") {
		t.Errorf("changes = %v", got)
	}
}

func TestChangesReportsControlsComingAndGoing(t *testing.T) {
	before := treeOf(textArea("Message", ""), Element{Role: "AXButton", Title: "Send", Enabled: true})
	after := treeOf(textArea("Message", ""), Element{Role: "AXButton", Title: "Undo", Enabled: true})

	got := strings.Join(Changes(before, after), "; ")
	if !strings.Contains(got, `"Undo" appeared`) || !strings.Contains(got, `"Send" is gone`) {
		t.Errorf("changes = %s", got)
	}
}

func TestNothingChangedIsSaidAsNothing(t *testing.T) {
	tree := treeOf(textArea("Message", "same"))
	if got := Changes(tree, tree); len(got) != 0 {
		t.Errorf("changes = %v, want none", got)
	}
}

// Typing is the case where the intent can actually be checked: the text either
// turned up in the window or it did not.
func TestLandedIsTrueWhenTheTextArrived(t *testing.T) {
	before := treeOf(textArea("Message", ""))
	after := treeOf(textArea("Message", "on my way"))
	p := Program{App: "Editor", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXTextArea", Title: "Message"}, Text: "on my way"},
	}}

	if !Landed(before, after, p) {
		t.Error("typed text arrived and was not confirmed")
	}
}

func TestLandedIsFalseWhenTheTextNeverArrived(t *testing.T) {
	before := treeOf(textArea("Message", ""))
	after := treeOf(textArea("Message", ""))
	p := Program{App: "Editor", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXTextArea", Title: "Message"}, Text: "on my way"},
	}}

	if Landed(before, after, p) {
		t.Error("nothing was typed and it was reported as done")
	}
}

// A click cannot be checked against an intent the way typing can, so the honest
// test is whether the window moved at all.
func TestAClickThatChangedNothingIsNotConfirmed(t *testing.T) {
	tree := treeOf(Element{Role: "AXButton", Title: "Send", Enabled: true})
	p := Program{App: "Editor", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Send"}},
	}}

	if Landed(tree, tree, p) {
		t.Error("a click that changed nothing was reported as done")
	}
}

func TestAClickThatChangedTheWindowIsConfirmed(t *testing.T) {
	before := treeOf(Element{Role: "AXButton", Title: "Send", Enabled: true})
	after := treeOf(Element{Role: "AXButton", Title: "Undo", Enabled: true})
	p := Program{App: "Editor", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Send"}},
	}}

	if !Landed(before, after, p) {
		t.Error("the window changed after the click and it was not confirmed")
	}
}

func TestAClickThatMovedFocusIsConfirmed(t *testing.T) {
	before := treeOf(Element{Role: "AXTextField", Title: "Search", Focused: false})
	after := treeOf(Element{Role: "AXTextField", Title: "Search", Focused: true})
	p := Program{App: "Editor", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXTextField", Title: "Search"}},
	}}

	if !Landed(before, after, p) {
		t.Error("a click that moved focus was not confirmed")
	}
}

func TestChangesReportsFocusChange(t *testing.T) {
	before := treeOf(Element{Role: "AXTextField", Title: "Search", Focused: false})
	after := treeOf(Element{Role: "AXTextField", Title: "Search", Focused: true})

	got := Changes(before, after)
	if len(got) != 1 || !strings.Contains(got[0], "now focused") {
		t.Errorf("changes = %v, want focus change", got)
	}
}

func TestChangesReportsWindowTitleChange(t *testing.T) {
	before := Tree{App: "Settings", Window: "General"}
	after := Tree{App: "Settings", Window: "Displays"}

	got := Changes(before, after)
	if len(got) != 1 || !strings.Contains(got[0], `window title changed to "Displays"`) {
		t.Errorf("changes = %v, want window title change", got)
	}
}

func trayWindow() Tree {
	return Tree{App: "Mail", Window: "Inbox", Elements: []Element{
		{Role: "AXTextField", Title: "Search", Enabled: true, Path: []int{0}},
		{Role: "AXStaticText", Title: "Battery", Value: "51%", Enabled: true, Path: []int{1}},
		{Role: "AXButton", Title: "Send", Enabled: true, Path: []int{2}},
	}}
}

// The coach used to move on because a battery percentage ticked over, skipping a
// step nobody had done.
func TestAWindowTickingOverIsNotSomebodyActing(t *testing.T) {
	before := trayWindow()
	after := trayWindow()
	after.Elements[1].Value = "50%"

	target := Selector{Role: "AXTextField", Title: "Search"}
	if ActedOn(before, after, target) {
		t.Error("a value changing elsewhere was read as the person acting")
	}
	if len(Changes(before, after)) == 0 {
		t.Error("the change itself still has to be describable to the step writer")
	}
}

func TestUsingTheControlPointedAtIsSomebodyActing(t *testing.T) {
	target := Selector{Role: "AXTextField", Title: "Search"}

	focused := trayWindow()
	focused.Elements[0].Focused = true
	if !ActedOn(trayWindow(), focused, target) {
		t.Error("the control being clicked into was not read as the person acting")
	}

	opened := trayWindow()
	opened.Elements = append(opened.Elements,
		Element{Role: "AXButton", Title: "Reply", Path: []int{3}},
		Element{Role: "AXButton", Title: "Forward", Path: []int{4}})
	if !ActedOn(trayWindow(), opened, target) {
		t.Error("a pane that filled with new controls was not read as the person acting")
	}
}
