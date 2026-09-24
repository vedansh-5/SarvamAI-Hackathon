package uiact

import "testing"

// System Settings names every sidebar item this way: no title, and the word on
// screen in the value. Two live runs died asking for "Wallpaper" while it sat in
// the sidebar, because nothing matched on a title that was never there.
func TestAControlIsFoundByTheNameItShows(t *testing.T) {
	tree := Tree{App: "System Settings", Elements: []Element{
		{Role: "AXStaticText", Title: "", Value: "Wallpaper"},
		{Role: "AXStaticText", Title: "", Value: "Displays"},
	}}

	found := tree.Find(Selector{Role: "AXStaticText", Title: "Wallpaper"})
	if len(found) != 1 {
		t.Fatalf("found %d, want 1", len(found))
	}
	if found[0].Value != "Wallpaper" {
		t.Errorf("found the wrong one: %+v", found[0])
	}
}

// A title still wins where there is one.
func TestATitleIsPreferredOverAValue(t *testing.T) {
	el := Element{Role: "AXStaticText", Title: "Send", Value: "some text it holds"}
	if got := el.Label(); got != "Send" {
		t.Errorf("Label() = %q, want Send", got)
	}
}

// The reason values are withheld has not gone away: a message is not a label.
func TestLongValuesAreNotLabels(t *testing.T) {
	long := Element{
		Role:  "AXStaticText",
		Value: "hey are we still on for tomorrow, I can do any time after two",
	}
	if got := long.Label(); got != "" {
		t.Errorf("Label() = %q, want empty — that is content, not a name", got)
	}

	multi := Element{Role: "AXStaticText", Value: "first line\nsecond line"}
	if got := multi.Label(); got != "" {
		t.Errorf("Label() = %q, want empty for multi-line content", got)
	}
}

// A text field's value is what someone typed, whatever its length.
func TestATextFieldValueIsNeverALabel(t *testing.T) {
	typed := Element{Role: "AXTextField", Value: "swordfish"}
	if got := typed.Label(); got != "" {
		t.Errorf("Label() = %q, want empty — a field's value is not its name", got)
	}
}

// The model is shown what it is allowed to ask for, so the name has to appear.
func TestTheNameAControlShowsIsInWhatTheModelReads(t *testing.T) {
	tree := Tree{App: "System Settings", Window: "System Settings", Elements: []Element{
		{Role: "AXStaticText", Title: "", Value: "Wallpaper"},
	}}

	text := tree.Text()
	if !contains(text, `AXStaticText "Wallpaper"`) {
		t.Errorf("the sidebar item is not addressable in what the model reads:\n%s", text)
	}
	if contains(text, "holds text") {
		t.Errorf("the name was also reported as content:\n%s", text)
	}
}

// A refused step is re-asked with the reason, and the reason has to name what was
// actually there (D-089).
func TestTheRefusalNamesWhatIsOnScreenUnderThatName(t *testing.T) {
	tree := Tree{App: "System Settings", Elements: []Element{
		{Role: "AXStaticText", Title: "", Value: "Wallpaper"},
	}}

	_, why := tree.resolve(Selector{Role: "AXButton", Title: "Wallpaper"})
	if why == "" {
		t.Fatal("an AXButton named Wallpaper was accepted, and there is none")
	}
	if !contains(why, "AXStaticText") {
		t.Errorf("the refusal does not say what is there instead: %q", why)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
