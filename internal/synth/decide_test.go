package synth

import (
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/route"
)

// The prompt is generated from route.Specs, so a capability added to the
// runtime is offered to the model without anyone remembering to edit a string.
func TestEveryKindIsOfferedToTheModel(t *testing.T) {
	p := BuildRoutePrompt(route.Request{
		Instruction:  "the top 10 stories from https://news.ycombinator.com",
		Shape:        "the top {n0} stories from {u0}",
		Placeholders: []string{"u0", "n0"},
	})

	for _, spec := range route.Specs {
		if !strings.Contains(p, string(spec.Kind)) {
			t.Errorf("kind %q was never offered", spec.Kind)
		}
		for _, slot := range spec.Slots {
			if !strings.Contains(p, slot.Name) {
				t.Errorf("%s slot %q was never described", spec.Kind, slot.Name)
			}
		}
	}

	if !strings.Contains(p, string(route.KindUnknown)) {
		t.Error("the model was never told it may decline")
	}
}

// Required and optional slots have to be distinguishable, or the model has no
// way to know which ones it must fill.
func TestSlotsSayWhetherTheyAreRequired(t *testing.T) {
	p := BuildRoutePrompt(route.Request{Shape: "do something"})
	if !strings.Contains(p, "(required)") || !strings.Contains(p, "(optional)") {
		t.Fatalf("prompt does not distinguish required from optional slots:\n%s", p)
	}
}

func TestAvailablePlaceholdersAreListed(t *testing.T) {
	p := BuildRoutePrompt(route.Request{
		Shape:        "the top {n0} stories from {u0}",
		Placeholders: []string{"u0", "n0"},
	})
	if !strings.Contains(p, "{u0}") || !strings.Contains(p, "{n0}") {
		t.Errorf("placeholders not listed:\n%s", p)
	}
}

// A shape with no values still has to produce a usable prompt rather than an
// empty section the model has to interpret.
func TestAShapeWithNoPlaceholdersSaysSo(t *testing.T) {
	p := BuildRoutePrompt(route.Request{Shape: "open my inbox"})
	if !strings.Contains(p, "none") {
		t.Errorf("empty placeholder list is not explained:\n%s", p)
	}
}

// Rung 2: the retry has to carry why the first attempt was rejected.
func TestARejectedAnswerIsQuotedBackOnTheRetry(t *testing.T) {
	p := BuildRoutePrompt(route.Request{
		Shape:    "the stories from {u0}",
		Failures: []string{`route: web.extract resolved with no url (template "{u9}" had nothing to fill from)`},
	})
	if !strings.Contains(p, "{u9}") {
		t.Errorf("the reason for rejection was not passed back:\n%s", p)
	}
}

// The schema's enum has to track route.Specs, or the model is constrained to
// answer with kinds that no longer exist — or forbidden from ones that do.
func TestTheSchemaOffersExactlyTheKnownKinds(t *testing.T) {
	schema := routeSchema()
	props := schema["properties"].(map[string]any)
	kind := props["kind"].(map[string]any)
	enum := kind["enum"].([]string)

	if len(enum) != len(route.Specs)+1 {
		t.Fatalf("enum has %d kinds, want %d specs plus unknown", len(enum), len(route.Specs))
	}
	got := map[string]bool{}
	for _, k := range enum {
		got[k] = true
	}
	for _, spec := range route.Specs {
		if !got[string(spec.Kind)] {
			t.Errorf("schema omits %q", spec.Kind)
		}
	}
	if !got[string(route.KindUnknown)] {
		t.Error("schema does not allow the model to decline")
	}
}
