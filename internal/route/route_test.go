package route

import "strings"

import "testing"

func TestADecisionFillsItsSlotsFromTheInstruction(t *testing.T) {
	shape, lits := Shape(`the top 30 stories from https://lobste.rs into a note called "Reading"`)

	d := Decision{
		Kind: KindWebExtract,
		Slots: map[string]string{
			"url":    "{u0}",
			"intent": "top {n0} stories",
			"title":  "{s0}",
		},
	}

	r, err := d.Resolve("irrelevant", shape, lits)
	if err != nil {
		t.Fatal(err)
	}
	if r.Slot("url") != "https://lobste.rs" {
		t.Errorf("url = %q", r.Slot("url"))
	}
	if r.Slot("intent") != "top 30 stories" {
		t.Errorf("intent = %q", r.Slot("intent"))
	}
	if r.Slot("title") != "Reading" {
		t.Errorf("title = %q", r.Slot("title"))
	}
}

// The point of the whole package: one decision, many instructions.
func TestOneDecisionServesEveryInstructionOfThatShape(t *testing.T) {
	d := Decision{
		Kind:  KindWebExtract,
		Slots: map[string]string{"url": "{u0}", "intent": "top {n0} stories"},
	}

	for _, instruction := range []string{
		"the top 10 stories from https://news.ycombinator.com",
		"the top 30 stories from https://lobste.rs",
		"The top 5 stories from example.com",
	} {
		shape, lits := Shape(instruction)
		r, err := d.Resolve(instruction, shape, lits)
		if err != nil {
			t.Fatalf("%q: %v", instruction, err)
		}
		if r.Slot("url") == "" || r.Slot("intent") == "" {
			t.Fatalf("%q resolved empty: %#v", instruction, r.Slots)
		}
	}
}

func TestADecisionMissingARequiredSlotIsRejected(t *testing.T) {
	d := Decision{Kind: KindWebExtract, Slots: map[string]string{"url": "{u0}"}}
	if err := d.Validate(); err == nil {
		t.Fatal("a web.extract with no intent was accepted")
	}
}

func TestAnInventedKindIsRejected(t *testing.T) {
	d := Decision{Kind: "web.hallucinate", Slots: map[string]string{"url": "{u0}"}}
	if err := d.Validate(); err == nil {
		t.Fatal("a kind the engine cannot run was accepted")
	}
}

// The failure this exists to catch: the decision is structurally fine, but
// this instruction has nothing to fill it from. Letting it through would send
// an empty URL to the browser and fail a long way from the cause.
func TestASlotThatFillsToNothingIsAMissNotARoute(t *testing.T) {
	shape, lits := Shape("the top stories")

	d := Decision{
		Kind:  KindWebExtract,
		Slots: map[string]string{"url": "{u0}", "intent": "top stories"},
	}

	_, err := d.Resolve("the top stories", shape, lits)
	if err == nil {
		t.Fatal("resolved with no URL to navigate to")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error should name the empty slot, got: %v", err)
	}
}

// Saying "I don't know" is a valid answer and must survive validation, so the
// router can surface it rather than being forced into a wrong kind.
func TestUnknownIsAllowedToCarryNoSlots(t *testing.T) {
	d := Decision{Kind: KindUnknown, Why: "no adapter reads PDFs yet"}
	if err := d.Validate(); err != nil {
		t.Fatalf("unknown was rejected: %v", err)
	}
	r, err := d.Resolve("summarise this PDF", "summarise this pdf", Literals{})
	if err != nil {
		t.Fatalf("unknown failed to resolve: %v", err)
	}
	if r.Kind != KindUnknown {
		t.Errorf("kind = %q", r.Kind)
	}
}

// The failure that caching by shape makes permanent: the model templates the
// number but hardcodes the site, so every later instruction of this shape asks
// the wrong page for the right thing — silently, and forever.
func TestATemplateThatHardcodesAValueIsRejected(t *testing.T) {
	shape, lits := Shape("the top 10 stories from https://news.ycombinator.com")

	d := Decision{
		Kind: KindWebExtract,
		Slots: map[string]string{
			"url":    "{u0}",
			"intent": "the top {n0} stories from https://news.ycombinator.com",
		},
	}

	_, err := d.Resolve("irrelevant", shape, lits)
	if err == nil {
		t.Fatal("a template with the URL baked into it was accepted")
	}
	if !strings.Contains(err.Error(), "u0") {
		t.Errorf("error should name the placeholder that was skipped, got: %v", err)
	}
}

// Short values are not evidence. A bare "10" turns up inside ordinary prose
// and inside longer numbers, and rejecting on it would make the check fire on
// correct templates.
func TestShortValuesDoNotTriggerTheLeakCheck(t *testing.T) {
	shape, lits := Shape("the top 10 stories from https://news.ycombinator.com")

	d := Decision{
		Kind:  KindWebExtract,
		Slots: map[string]string{"url": "{u0}", "intent": "top {n0} stories"},
	}
	if _, err := d.Resolve("irrelevant", shape, lits); err != nil {
		t.Fatalf("a correct template was rejected: %v", err)
	}
}

// Word boundaries, so a literal "100" does not match inside "1000" and a
// template mentioning "10" is not read as leaking a literal "100".
func TestTheLeakCheckRespectsWordBoundaries(t *testing.T) {
	shape, lits := Shape("the top 100 stories from example.com")

	d := Decision{
		Kind:  KindWebExtract,
		Slots: map[string]string{"url": "{u0}", "intent": "top {n0} of the 1000 stories"},
	}
	if _, err := d.Resolve("irrelevant", shape, lits); err != nil {
		t.Fatalf("100 was matched inside 1000: %v", err)
	}
}
