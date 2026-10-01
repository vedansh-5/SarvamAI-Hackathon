package uiact

import "testing"

func TestWhichStepsCannotBeTakenBack(t *testing.T) {
	button := func(title string) Element { return Element{Role: "AXButton", Title: title, Enabled: true} }
	click := func(title string) Step {
		return Step{Do: "click", Target: Selector{Role: "AXButton", Title: title}}
	}

	for _, title := range []string{"Pay now", "Pay ₹4,200", "Place order", "Confirm booking", "Send", "Publish", "Post", "Delete account", "Submit", "Buy Now", "Confirm and pay"} {
		if final, _ := Commits(click(title), button(title)); !final {
			t.Errorf("%q was not treated as a step that cannot be taken back", title)
		}
	}
	for _, title := range []string{"Search flights", "Next", "Continue", "Postpone", "Payment options", "Senders", "Flights", "Add to cart", "Sign in", "Upload"} {
		if final, why := Commits(click(title), button(title)); final {
			t.Errorf("%q was treated as final: %s", title, why)
		}
	}

	marked := click("Continue")
	marked.Final = true
	if final, _ := Commits(marked, button("Continue")); !final {
		t.Error("a step the model marked final was not treated as one")
	}

	message := Element{Role: "AXTextArea", Title: "Type a message", Enabled: true}
	enter := Step{Do: "press", Key: "Enter", Target: Selector{Role: "AXTextArea", Title: "Type a message"}}
	if final, _ := Commits(enter, message); !final {
		t.Error("Enter in a message box was not treated as sending")
	}
	search := Element{Role: "AXTextField", Title: "Search", Enabled: true}
	if final, _ := Commits(Step{Do: "press", Key: "Enter"}, search); final {
		t.Error("Enter in a search box was treated as final")
	}
	if final, _ := Commits(Step{Do: "type", Text: "send the report"}, message); final {
		t.Error("typing was treated as final because of the words typed")
	}
	if final, _ := Commits(Step{Do: "type", Text: "hello", Final: true}, message); final {
		t.Error("typing was treated as final because the model marked it; typing sends nothing")
	}
}

func TestAHiddenMarkInAControlsNameDoesNotMakeItADifferentControl(t *testing.T) {
	want := Element{Role: "AXTextArea", Title: "Compose message"}
	if !sameControl(want, "AXTextArea", "\u200eCompose message") {
		t.Error("the control under the pointer was rejected for a character nobody can see")
	}
	if sameControl(want, "AXTextArea", "Search") {
		t.Error("a different control was accepted")
	}
}
