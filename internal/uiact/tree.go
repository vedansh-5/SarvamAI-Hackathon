package uiact

import (
	"fmt"
	"strings"
)

type Element struct {
	Role     string   `json:"role"`
	Title    string   `json:"title,omitempty"`
	Value    string   `json:"value,omitempty"`
	Enabled  bool     `json:"enabled"`
	Focused  bool     `json:"focused,omitempty"`
	Position []int    `json:"position,omitempty"`
	Size     []int    `json:"size,omitempty"`
	Actions  []string `json:"actions,omitempty"`
	Path     []int    `json:"path"`
}

type Tree struct {
	App      string    `json:"app"`
	Window   string    `json:"window"`
	Elements []Element `json:"elements"`
}

const MaxTreeBytes = 24000

var typeable = map[string]bool{
	"textfield":   true,
	"textarea":    true,
	"combobox":    true,
	"searchfield": true,
}

func role(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "AX")
	return strings.ToLower(s)
}

func (e Element) Typeable() bool { return typeable[role(e.Role)] }

func (e Element) Pressable() bool {
	if len(e.Actions) == 0 {
		return true
	}
	for _, a := range e.Actions {
		if strings.EqualFold(a, "AXPress") {
			return true
		}
	}
	return false
}

// MaxLabelLength is how long a value may be and still be read as a label rather
// than as content. A sidebar item is a word or two; a message is not.
const MaxLabelLength = 40

// labelRoles name the roles whose value is what the control is called, rather
// than something a person typed or received.
var labelRoles = map[string]bool{"AXStaticText": true}

// Label is what the element is called on screen.
//
// Usually the title, but a great many controls have none and carry their name in
// the value instead. System Settings is the case that found this: every item in
// its sidebar is an AXStaticText with an empty title and the value "Wallpaper",
// "Displays", "Sound". Values are withheld from what the model is shown, for good
// reason — a text field holds what someone typed — so the whole sidebar arrived
// as a list of untitled AXStaticText, invisible and unaddressable. Two live runs
// died asking for a control that was on screen the entire time.
//
// Short values on labelling roles only. A long one is content, and the reason
// values are withheld has not gone away.
func (e Element) Label() string {
	if title := strings.TrimSpace(e.Title); title != "" {
		return title
	}

	value := strings.TrimSpace(e.Value)
	if !labelRoles[e.Role] || value == "" {
		return ""
	}
	if strings.Contains(value, "\n") || len([]rune(value)) > MaxLabelLength {
		return ""
	}
	return value
}

func (t Tree) Find(sel Selector) []Element {
	var out []Element
	for _, el := range t.Elements {
		if role(el.Role) != role(sel.Role) {
			continue
		}
		if sel.Title != "" && !strings.EqualFold(el.Label(), strings.TrimSpace(sel.Title)) {
			continue
		}
		out = append(out, el)
	}
	return out
}

func (t Tree) Text() string { return t.TextWithin(MaxTreeBytes) }

func (t Tree) TextWithin(budget int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — window %q\n\n", t.App, t.Window)

	counts := map[string]int{}
	for _, el := range t.Elements {
		key := role(el.Role) + "\x00" + strings.ToLower(el.Label())
		counts[key]++
	}

	seen := map[string]int{}
	for _, el := range t.Elements {
		label := el.Label()
		key := role(el.Role) + "\x00" + strings.ToLower(label)
		seen[key]++

		line := "  " + el.Role
		if label != "" {
			line += fmt.Sprintf(" %q", label)
		}
		if counts[key] > 1 {
			line += fmt.Sprintf(" (index %d of %d)", seen[key], counts[key])
		}
		// Only when the value is not already the name above, or every sidebar
		// item would read as holding text it does not hold.
		if el.Value != "" && label != strings.TrimSpace(el.Value) {
			if el.Focused {
				line += fmt.Sprintf(" value=%q", truncate(el.Value, 60))
			} else {
				line += " (holds text)"
			}
		}
		if !el.Enabled {
			line += " [disabled]"
		}
		line += "\n"

		if b.Len()+len(line) > budget {
			b.WriteString("  … more elements than fit here\n")
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func samePath(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (t Tree) SelectorFor(el Element) Selector {
	sel := Selector{Role: el.Role, Title: el.Title}

	var seen, total int
	for _, other := range t.Elements {
		if role(other.Role) != role(el.Role) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(other.Title), strings.TrimSpace(el.Title)) {
			continue
		}
		total++
		if samePath(other.Path, el.Path) {
			seen = total
		}
	}
	if total > 1 && seen > 0 {
		sel.Index = seen
	}
	return sel
}

func (t Tree) TypingTarget() (Element, string) {
	var typeables []Element
	for _, el := range t.Elements {
		if !el.Typeable() || !el.Enabled {
			continue
		}
		if el.Focused {
			return el, ""
		}
		typeables = append(typeables, el)
	}

	switch len(typeables) {
	case 0:
		return Element{}, "nothing on screen can hold typed text"
	case 1:
		return typeables[0], ""
	default:
		return Element{}, fmt.Sprintf(
			"%d controls can hold typed text and none of them has the cursor, "+
				"so there is no way to say which one was meant", len(typeables))
	}
}

func (e Element) Center() (x, y int, ok bool) {
	if len(e.Position) != 2 || len(e.Size) != 2 {
		return 0, 0, false
	}
	if e.Size[0] <= 0 || e.Size[1] <= 0 {
		return 0, 0, false
	}
	return e.Position[0] + e.Size[0]/2, e.Position[1] + e.Size[1]/2, true
}

type box struct{ x0, y0, x1, y1 int }

func (b box) valid() bool { return b.x1 > b.x0 && b.y1 > b.y0 }

func (b box) clip(o box) box {
	if o.x0 > b.x0 {
		b.x0 = o.x0
	}
	if o.y0 > b.y0 {
		b.y0 = o.y0
	}
	if o.x1 < b.x1 {
		b.x1 = o.x1
	}
	if o.y1 < b.y1 {
		b.y1 = o.y1
	}
	return b
}

func (e Element) box() (box, bool) {
	if len(e.Position) != 2 || len(e.Size) != 2 {
		return box{}, false
	}
	if e.Size[0] <= 0 || e.Size[1] <= 0 {
		return box{}, false
	}
	return box{
		e.Position[0], e.Position[1],
		e.Position[0] + e.Size[0], e.Position[1] + e.Size[1],
	}, true
}

var displays = displayBounds

func encloses(prefix, path []int) bool {
	if len(prefix) >= len(path) {
		return false
	}
	for i := range prefix {
		if prefix[i] != path[i] {
			return false
		}
	}
	return true
}

func (t Tree) visibleBox(el Element) (box, bool) {
	b, ok := el.box()
	if !ok {
		return box{}, false
	}
	for _, a := range t.Elements {
		if !encloses(a.Path, el.Path) {
			continue
		}
		if ab, ok := a.box(); ok {
			b = b.clip(ab)
		}
	}
	if d, ok := displays(); ok {
		b = b.clip(d)
	}
	if !b.valid() {
		return box{}, false
	}
	return b, true
}

func (t Tree) ClickPoint(el Element) (x, y int, ok bool) {
	b, ok := t.visibleBox(el)
	if !ok {
		return 0, 0, false
	}
	return b.x0 + (b.x1-b.x0)/2, b.y0 + (b.y1-b.y0)/2, true
}

func (t Tree) RectFor(sel Selector) (x, y, w, h int, why string) {
	el, why := t.resolve(sel)
	if why != "" {
		return 0, 0, 0, 0, why
	}
	b, ok := t.visibleBox(el)
	if !ok {
		return 0, 0, 0, 0, fmt.Sprintf("no part of the %s %q is visible on screen", el.Role, el.Title)
	}
	return b.x0, b.y0, b.x1 - b.x0, b.y1 - b.y0, ""
}
