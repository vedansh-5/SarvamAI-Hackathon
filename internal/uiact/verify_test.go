package uiact

import (
	"strings"
	"testing"
)

func withLookup(t *testing.T, role, title string, owned bool) {
	t.Helper()
	original := lookupAt
	lookupAt = func(int, int, int) (string, string, bool, error) {
		return role, title, owned, nil
	}
	t.Cleanup(func() { lookupAt = original })
}

func withFrontOwner(t *testing.T, pid int, name string, known bool) {
	t.Helper()
	original := frontOwnerAt
	frontOwnerAt = func(int, int) (int, string, bool) { return pid, name, known }
	t.Cleanup(func() { frontOwnerAt = original })
}

func clickSend() Program {
	return Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Send"}},
	}}
}

func TestVerifyAcceptsTheControlItExpects(t *testing.T) {
	withFrontOwner(t, 42, "Telegram", true)
	withLookup(t, "AXButton", "Send", true)
	if err := VerifyTargets(42, clickSend(), window()); err != nil {
		t.Errorf("refused a matching control: %v", err)
	}
}

func TestVerifyRefusesADifferentControl(t *testing.T) {
	withFrontOwner(t, 42, "Telegram", true)
	withLookup(t, "AXButton", "Delete", true)
	err := VerifyTargets(42, clickSend(), window())
	if err == nil {
		t.Fatal("clicked a control that is not the one planned")
	}
	if !strings.Contains(err.Error(), "found a AXButton \"Delete\"") {
		t.Errorf("err = %v", err)
	}
}

// Which window is on top is the question the accessibility hit test cannot
// answer while Stage Manager is running, so it is asked of the window list.
func TestVerifyRefusesWhenAnotherApplicationIsInFront(t *testing.T) {
	withFrontOwner(t, 99, "Code", true)
	withLookup(t, "AXButton", "Send", true)
	err := VerifyTargets(42, clickSend(), window())
	if err == nil {
		t.Fatal("clicked through another application's window")
	}
	if !strings.Contains(err.Error(), "inside Code's window") {
		t.Errorf("err = %v", err)
	}
}

// Stage Manager answers the hit test with its own element rather than the
// application's. That costs the control check, and must not cost the click:
// the window list has already said the point belongs to the right application.
func TestVerifyStillActsWhenTheHitTestIsNotTheApplications(t *testing.T) {
	withFrontOwner(t, 42, "Telegram", true)
	withLookup(t, "AXGroup", "", false)
	if err := VerifyTargets(42, clickSend(), window()); err != nil {
		t.Errorf("refused a click the window list vouched for: %v", err)
	}
}

func TestTreeTextWithholdsValuesExceptWhereTheCursorIs(t *testing.T) {
	tree := Tree{App: "Notes", Window: "Notes", Elements: []Element{
		{Role: "AXTextArea", Title: "Body", Value: "private note contents", Enabled: true},
		{Role: "AXTextField", Title: "Search", Value: "typed query", Enabled: true, Focused: true},
	}}

	got := tree.Text()
	if strings.Contains(got, "private note contents") {
		t.Errorf("the contents of an unfocused control went into the prompt:\n%s", got)
	}
	if !strings.Contains(got, "(holds text)") {
		t.Errorf("a control holding text was not mentioned at all:\n%s", got)
	}
	if !strings.Contains(got, "typed query") {
		t.Errorf("the focused control's value was withheld, which the model needs:\n%s", got)
	}
}
