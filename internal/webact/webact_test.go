package webact

import (
	"errors"
	"strings"
	"testing"
)

const page = `<html><body>
  <form id="composer">
    <input id="msg" type="text" name="message">
    <textarea id="notes"></textarea>
    <div id="rich" contenteditable="true"></div>
    <button id="send" type="submit">Send</button>
  </form>
  <button class="row-action">Open</button>
  <button class="row-action">Open</button>
  <span id="label">Not typeable</span>
</body></html>`

func reasons(t *testing.T, err error) []string {
	t.Helper()
	var invalid *InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidError", err)
	}
	return invalid.Reasons
}

func mentions(rs []string, want string) bool {
	for _, r := range rs {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}

func send() Program {
	return Program{
		URL: "https://example.test/chat",
		Steps: []Step{
			{Do: "type", Selector: "#msg", Text: "on my way", Describe: "Type the message"},
			{Do: "click", Selector: "#send", Describe: "Send it"},
		},
	}
}

func TestAValidProgramRenders(t *testing.T) {
	n, _ := Parse(page)
	script, err := Render(send(), n)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(script, `typeInto("#msg", "on my way")`) {
		t.Errorf("the typing step did not render:\n%s", script)
	}
	if !strings.Contains(script, `click("#send")`) {
		t.Errorf("the click step did not render:\n%s", script)
	}
}

// The failure a confirmation cannot catch: the plan says "click Open", four
// of them match, and the user confirms one thing while a different one
// happens.
func TestAnAmbiguousSelectorIsRefused(t *testing.T) {
	n, _ := Parse(page)
	p := Program{Steps: []Step{{Do: "click", Selector: ".row-action"}}}

	if !mentions(reasons(t, Validate(p, n)), "does not say which one") {
		t.Error("a selector matching several elements was accepted")
	}
}

func TestASelectorThatMatchesNothingIsRefused(t *testing.T) {
	n, _ := Parse(page)
	p := Program{Steps: []Step{{Do: "click", Selector: "#nowhere"}}}

	if !mentions(reasons(t, Validate(p, n)), "nothing on the page matches") {
		t.Error("a selector matching nothing was accepted")
	}
}

// Typing into something that cannot hold text does nothing at all, which is
// worse than failing: the plan is confirmed, the click lands, the message is
// empty.
func TestTypingIntoSomethingThatCannotHoldTextIsRefused(t *testing.T) {
	n, _ := Parse(page)
	p := Program{Steps: []Step{{Do: "type", Selector: "#label", Text: "hello"}}}

	if !mentions(reasons(t, Validate(p, n)), "cannot be typed into") {
		t.Error("typing into a span was accepted")
	}
}

func TestContentEditableIsTypeable(t *testing.T) {
	n, _ := Parse(page)
	p := Program{Steps: []Step{{Do: "type", Selector: "#rich", Text: "hello"}}}

	if err := Validate(p, n); err != nil {
		t.Errorf("a contenteditable div was refused: %v", err)
	}
}

func TestOnlyKnownVerbsAndKeys(t *testing.T) {
	n, _ := Parse(page)

	bad := Program{Steps: []Step{{Do: "navigate", Selector: "#send"}}}
	if !mentions(reasons(t, Validate(bad, n)), "not something Vspk does") {
		t.Error("an invented verb was accepted")
	}

	key := Program{Steps: []Step{{Do: "press", Selector: "#msg", Key: "F13"}}}
	if !mentions(reasons(t, Validate(key, n)), "not a key Vspk presses") {
		t.Error("an arbitrary key was accepted")
	}
}

// Every web program is confirmed. A page declares nothing, so there is no
// dictionary to ask whether a click sends a message or opens a menu.
func TestEveryProgramCountsAsAWrite(t *testing.T) {
	if !send().Writes() {
		t.Error("a program that types and clicks is not reported as a write")
	}
}

func TestValuesTypedInAreCaught(t *testing.T) {
	literals := map[string]string{"s0": "on my way"}

	if got := Baked(send(), literals); len(got) != 1 {
		t.Errorf("Baked = %v, want the hardcoded message", got)
	}

	templated := send()
	templated.Steps[0].Text = "{s0}"
	if got := Baked(templated, literals); len(got) != 0 {
		t.Errorf("Baked = %v, want nothing", got)
	}
}

func TestFillingChangesTextAndNotSelectors(t *testing.T) {
	p := send()
	p.Steps[0].Text = "{s0}"

	got := p.Fill(func(v string) string {
		if v == "{s0}" {
			return "running late"
		}
		return v
	})

	if got.Steps[0].Text != "running late" {
		t.Errorf("text = %q", got.Steps[0].Text)
	}
	if got.Steps[0].Selector != "#msg" || got.Steps[1].Selector != "#send" {
		t.Error("filling changed a selector")
	}
}

func TestAProgramCannotBeUnboundedlyLong(t *testing.T) {
	n, _ := Parse(page)
	p := Program{}
	for i := 0; i <= MaxSteps; i++ {
		p.Steps = append(p.Steps, Step{Do: "click", Selector: "#send"})
	}
	if !mentions(reasons(t, Validate(p, n)), "is the most a single request may take") {
		t.Error("an unbounded program was accepted")
	}
}
