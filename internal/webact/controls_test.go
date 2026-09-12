package webact

import (
	"strings"
	"testing"
)

const privatePage = `<html><body>
  <h1>Dinner on Thursday</h1>
  <p>the account number is 4417 9923 1102</p>
  <form id="composer">
    <input id="msg" type="text" name="message" placeholder="Type a message" value="see you at the usual place">
    <textarea name="notes" aria-label="Notes">my private notes</textarea>
    <button id="send" type="submit">Send</button>
  </form>
  <a href="/archive">Archive</a>
  <div contenteditable="true" aria-label="Rich composer">drafted text nobody should see</div>
</body></html>`

func controlsFor(t *testing.T, src string) string {
	t.Helper()
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return ControlsText(Controls(doc), MaxControlsBytes)
}

func TestControlsCarryNoPageContent(t *testing.T) {
	got := controlsFor(t, privatePage)

	for _, secret := range []string{
		"account number",
		"4417 9923 1102",
		"see you at the usual place",
		"my private notes",
		"drafted text nobody should see",
		"Dinner on Thursday",
	} {
		if strings.Contains(got, secret) {
			t.Errorf("page content reached the prompt: %q\n%s", secret, got)
		}
	}
}

func TestControlsCarryWhatIsNeededToChooseOne(t *testing.T) {
	got := controlsFor(t, privatePage)

	for _, want := range []string{"#msg", "#send", "Type a message", "Notes", "Rich composer"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q from the controls listing:\n%s", want, got)
		}
	}
}

func TestEveryOfferedSelectorResolvesToExactlyOneControl(t *testing.T) {
	doc, err := Parse(privatePage)
	if err != nil {
		t.Fatal(err)
	}

	found := Controls(doc)
	if len(found) == 0 {
		t.Fatal("no controls found at all")
	}
	for _, c := range found {
		if !matchesOne(doc, c.Selector) {
			t.Errorf("selector %q does not address exactly one control", c.Selector)
		}
	}
}

func TestAButtonsOwnWordsAreItsLabel(t *testing.T) {
	got := controlsFor(t, privatePage)
	if !strings.Contains(got, "Send") {
		t.Errorf("a button's text is how a person names it:\n%s", got)
	}
}
