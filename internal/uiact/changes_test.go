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
