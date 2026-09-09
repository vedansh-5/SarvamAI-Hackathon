package route

import (
	"fmt"
	"sort"
	"strings"
)

// Kind is what sort of work an instruction is asking for.
//
// Kinds are the rungs of the interaction ladder (D-009, D-010) made
// selectable. Adding one is how a new capability becomes reachable from the
// overlay; until a kind exists for it, there is no way to ask.
type Kind string

const (
	// KindWebExtract is V1: read records off a page, write them somewhere.
	KindWebExtract Kind = "web.extract"

	// KindAppAction is Phase 3: do something in a running application.
	KindAppAction Kind = "app.act"

	// KindUnknown is the honest answer. The router is allowed to say it does
	// not know, and saying so is better than routing to the nearest kind and
	// failing somewhere the user cannot interpret.
	KindUnknown Kind = "unknown"
)

// Slot is one parameter a kind needs.
type Slot struct {
	Name     string
	Describe string
	Required bool
}

// Spec is everything known about a kind: what it does and what it needs.
//
// One definition, two consumers — the prompt is generated from this and the
// validator checks against it. Describing the kinds to the model in one place
// and enforcing them in another is how a router ends up rejecting exactly the
// answers it asked for.
type Spec struct {
	Kind     Kind
	Describe string
	Slots    []Slot
}

// Specs are the kinds the model may choose from, in the order it is offered
// them. Adding a capability to VANS means adding a Spec: until one exists,
// there is no way for a user to ask for it.
var Specs = []Spec{
	{
		Kind:     KindWebExtract,
		Describe: "Read a set of repeating records off a web page and write them somewhere.",
		Slots: []Slot{
			{"url", "the page to read, as an address", true},
			{"intent", "what to pull out of it, in plain words", true},
			{"title", "what to call the note that gets written", false},
		},
	},
	{
		Kind:     KindAppAction,
		Describe: "Do something inside a running macOS application, the way a person would.",
		Slots: []Slot{
			{"app", "the application to act in", true},
			{"action", "what to do there, in plain words", true},
			{"target", "the specific thing to act on, if the instruction names one", false},
		},
	},
}

func specOf(k Kind) (Spec, bool) {
	for _, s := range Specs {
		if s.Kind == k {
			return s, true
		}
	}
	return Spec{}, false
}

// requiredSlots lists the slots a kind cannot do without.
func requiredSlots(k Kind) []string {
	spec, ok := specOf(k)
	if !ok {
		return nil
	}
	var out []string
	for _, s := range spec.Slots {
		if s.Required {
			out = append(out, s.Name)
		}
	}
	return out
}

func (k Kind) valid() bool {
	if k == KindUnknown {
		return true
	}
	_, ok := specOf(k)
	return ok
}

// Decision is what the model writes: which kind, and how to build that kind's
// slots out of whatever instruction arrives next.
//
// Slot values are templates over the placeholders in the shape, not finished
// strings — that is what makes the decision reusable. "{u0}" means "whatever
// URL this instruction happens to carry", so the same decision serves every
// instruction of the same shape.
type Decision struct {
	Kind  Kind              `json:"kind"`
	Slots map[string]string `json:"slots"`

	// Why is the model's one-line account of the choice. It is shown to the
	// user when the router declines, and stored either way: a wrong decision
	// that explains itself can be corrected, one that does not has to be
	// guessed at.
	Why string `json:"why"`
}

// Route is a Decision with this instruction's values filled in.
type Route struct {
	Kind        Kind
	Slots       map[string]string
	Why         string
	Shape       string
	Instruction string

	// FromCache reports that no model call was made. The number worth
	// watching, and the same claim the extraction path makes (D-006).
	FromCache bool
}

// Slot reads a filled slot.
func (r Route) Slot(name string) string { return r.Slots[name] }

// Validate rejects a decision the engine could not act on.
//
// This runs on cache reads as well as fresh model output. A decision stored
// before a kind's required slots changed is exactly as unusable as one the
// model just got wrong, and the store is not the place to discover that.
func (d Decision) Validate() error {
	if !d.Kind.valid() {
		return fmt.Errorf("route: unknown kind %q", d.Kind)
	}
	if d.Kind == KindUnknown {
		return nil
	}
	var missing []string
	for _, slot := range requiredSlots(d.Kind) {
		if strings.TrimSpace(d.Slots[slot]) == "" {
			missing = append(missing, slot)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("route: %s needs %s", d.Kind, strings.Join(missing, ", "))
	}
	return nil
}

// Resolve fills a decision's templates with one instruction's literals.
//
// A slot that fills to nothing is dropped rather than passed on empty, so a
// caller checking for a slot's presence gets a truthful answer instead of a
// blank that looks deliberate.
func (d Decision) Resolve(instruction, shape string, lits Literals) (Route, error) {
	if err := d.Validate(); err != nil {
		return Route{}, err
	}

	slots := make(map[string]string, len(d.Slots))
	for name, tmpl := range d.Slots {
		if v := Fill(tmpl, lits); v != "" {
			slots[name] = v
		}
	}

	r := Route{
		Kind:        d.Kind,
		Slots:       slots,
		Why:         d.Why,
		Shape:       shape,
		Instruction: instruction,
	}

	// Re-checked after filling, not only before. A decision can be structurally
	// sound and still resolve to nothing — the template referenced {u0} and
	// this instruction carried no URL — and that is a miss, not a route.
	if d.Kind != KindUnknown {
		for _, slot := range requiredSlots(d.Kind) {
			if slots[slot] == "" {
				return Route{}, fmt.Errorf(
					"route: %s resolved with no %s (template %q had nothing to fill from)",
					d.Kind, slot, d.Slots[slot])
			}
		}
	}
	return r, nil
}
