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

func clickSend() Program {
	return Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Send"}},
	}}
}

func TestVerifyAcceptsTheControlItExpects(t *testing.T) {
	withLookup(t, "AXButton", "Send", true)
	if err := VerifyTargets(42, clickSend(), window()); err != nil {
		t.Errorf("refused a matching control: %v", err)
	}
}

func TestVerifyRefusesADifferentControl(t *testing.T) {
	withLookup(t, "AXButton", "Delete", true)
	err := VerifyTargets(42, clickSend(), window())
	if err == nil {
		t.Fatal("clicked a control that is not the one planned")
	}
	if !strings.Contains(err.Error(), "found a AXButton \"Delete\"") {
		t.Errorf("err = %v", err)
	}
}

func TestVerifyRefusesWhenAnotherApplicationIsInTheWay(t *testing.T) {
	withLookup(t, "AXButton", "Send", false)
	err := VerifyTargets(42, clickSend(), window())
	if err == nil {
		t.Fatal("clicked through another application's window")
	}
	if !strings.Contains(err.Error(), "covered by another application") {
		t.Errorf("err = %v", err)
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
