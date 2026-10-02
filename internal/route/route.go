package route

import (
	"crypto/sha256"
	"encoding/hex"
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

	// KindWebSearch is a question with no page attached: find the page first,
	// then read it. Its own kind rather than web.extract with a cleverer URL
	// slot, because what the user supplies is a phrase, not an address, and a
	// slot that sometimes holds one and sometimes the other is a slot nothing
	// can validate.
	KindWebSearch Kind = "web.search"

	// KindPageAction is doing something on the page already on screen —
	// typing into it, clicking something in it. The counterpart of
	// KindPageHere, which only reads.
	//
	// Its own kind because the two need different things said about them: one
	// shows an answer, the other asks before it touches anything.
	KindPageAction Kind = "page.act"

	// KindAppAction is Phase 3: do something in a running application.
	KindAppAction Kind = "app.act"

	// KindPageHere is the page already on screen. "Summarise this", "what does
	// this say", "pull the prices off this" — a request with no address in it
	// because the address is whatever the user is looking at.
	//
	// Its own kind rather than web.extract with a missing url, because a
	// missing required slot is a rejected decision and this one is complete:
	// the user did supply the page, by having it open.
	KindPageHere Kind = "page.here"

	// KindPageStyle restyles the page already on screen so that what the user
	// asked about is visible on it. It changes how the page looks and nothing
	// else — no navigation, no typing, no clicking.
	KindPageStyle Kind = "page.style"

	// KindAppGuide shows the user what to do rather than doing it: the control
	// to use is pointed at on screen, a step at a time.
	KindAppGuide Kind = "app.guide"

	KindText Kind = "text.work"

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
		Kind:     KindWebSearch,
		Describe: "Search the web for something and read the results back.",
		Slots: []Slot{
			{"query", "what to search for, as the words to put in the box", true},
			{"engine", "which search engine, only if the instruction names one", false},
		},
	},
	{
		Kind:     KindPageHere,
		Describe: "Read the page the user already has open, without going anywhere.",
		Slots: []Slot{
			{"intent", "what to get from the page they are looking at, in plain words", true},
		},
	},
	{
		Kind:     KindText,
		Describe: "Work on a piece of text and give the result back as words, touching nothing on screen: translate it, rewrite or shorten it, fix its spelling and grammar, summarise or explain it, or draft a reply to it. The text is what the user has selected, or what they wrote in the instruction itself (\"translate this to Hindi\", \"make this sound more formal\", \"what does this mean\"). Use this whenever what is wanted back is words rather than an action. Never app.act or page.act for these: no application is being asked to do anything.",
		Slots: []Slot{
			{"intent", "what to do with the text, in plain words", true},
		},
	},
	{
		Kind:     KindPageAction,
		Describe: "Do something on a website, the way a person would: on the page the user has open, or on a site they name. Covers single actions (type into it, click something) and tasks of many steps (search and choose, fill in a form, book something, upload something). Use this whenever the thing being acted on is a website rather than a Mac application.",
		Slots: []Slot{
			{"action", "what to do there, in plain words", true},
			{"site", "the website named in the instruction, as a name or an address (makemytrip, youtube.com); empty when it is the page already open", false},
		},
	},
	{
		Kind:     KindPageStyle,
		Describe: "Change how the web page open in the browser looks: tint, highlight, dim or colour parts of it so they stand out, or clear that styling again. Web pages only, and only about appearance — never about where something is or how to do something.",
		Slots: []Slot{
			{"intent", "what to make stand out, or that the highlighting should be cleared, in plain words", true},
		},
	},
	{
		Kind:     KindAppGuide,
		Describe: "Answer a question about using the Mac or a website by pointing at the control on screen: where something is, how to do something, which button to press. The user is shown what to do and does it themselves. Use this for \"where is\", \"how do I\", \"show me how\" and \"show me where\". The question does not have to be about the window in front — working out which application or site it belongs to happens afterwards, so do not decline because the task lives somewhere else.",
		Slots: []Slot{
			{"intent", "what they want to do or find, in plain words", true},
		},
	},
	{
		Kind:     KindAppAction,
		Describe: "Do something inside a macOS application, the way a person would, by clicking and typing in its window: send a message in WhatsApp or Messages, add a reminder, play a song, type into a document, change a setting. Any application counts, named or in front. Not for websites: something done on a site is page.act.",
		Slots: []Slot{
			{"app", "the application to act in, if named; otherwise the application in front", false},
			{"action", "what to do there, in plain words", true},
			{"target", "the specific thing to act on, if the instruction names one", false},
		},
	},
}

// A decision is only as good as the list of kinds it was made against, so the
// list is fingerprinted and stored with it. Change a description and every
// decision taken under the old one is dropped rather than replayed.
func SpecsFingerprint() string {
	h := sha256.New()
	for _, spec := range Specs {
		fmt.Fprintf(h, "%s|%s|", spec.Kind, spec.Describe)
		for _, slot := range spec.Slots {
			fmt.Fprintf(h, "%s,%s,%v;", slot.Name, slot.Describe, slot.Required)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
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

	// Templates are the slots before filling, and Literals what filled them.
	// Carried because anything cached under the shape has to be written
	// against the templates and filled at use, or it bakes in the values of
	// whichever instruction happened to be first (D-039).
	Templates map[string]string
	Literals  Literals

	// FromCache reports that no model call was made. The number worth
	// watching, and the same claim the extraction path makes (D-006).
	FromCache bool
}

// Slot reads a filled slot.
func (r Route) Slot(name string) string { return r.Slots[name] }

// Template reads a slot before filling, which is what anything cached by
// shape must be written against.
func (r Route) Template(name string) string { return r.Templates[name] }

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

// leaked reports literal values a template baked in instead of referencing.
//
// This is the failure that caching by shape makes permanent. Writing the
// decision while looking at one instruction, a model will happily produce
// "the top {n0} stories from the Hacker News homepage" — templating the number
// but hardcoding the site. That is correct for the instruction in front of it
// and wrong for every later one of the same shape, and because the decision is
// cached, the wrongness is durable and silent: lobste.rs gets asked for Hacker
// News stories forever.
//
// So it is a floor rather than an instruction. The prompt asks for templates;
// this refuses anything else.
func leaked(tmpl string, lits Literals) []string {
	var found []string
	for key, value := range lits {
		v := strings.TrimSpace(value)
		// Short values are not evidence of anything — a bare "5" appears
		// inside ordinary prose and inside longer numbers.
		if len(v) < 3 {
			continue
		}
		if containsWord(strings.ToLower(tmpl), strings.ToLower(v)) {
			found = append(found, fmt.Sprintf("%q (should be {%s})", v, key))
		}
	}
	sort.Strings(found)
	return found
}

// containsWord reports whether needle appears in haystack on word boundaries,
// so "10" does not match inside "100".
func containsWord(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] != needle {
			continue
		}
		if i > 0 && isWordish(rune(haystack[i-1])) {
			continue
		}
		if end := i + len(needle); end < len(haystack) && isWordish(rune(haystack[end])) {
			continue
		}
		return true
	}
	return false
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
		if bad := leaked(tmpl, lits); len(bad) > 0 {
			return Route{}, fmt.Errorf(
				"route: slot %q hardcodes this instruction's values instead of referencing them: %s — "+
					"the decision is reused for every instruction of this shape, so those must be placeholders",
				name, strings.Join(bad, ", "))
		}
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
		Templates:   d.Slots,
		Literals:    lits,
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
