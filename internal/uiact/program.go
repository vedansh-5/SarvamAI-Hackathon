package uiact

import (
	"sort"
	"strings"
)

type Program struct {
	App   string `json:"app"`
	Steps []Step `json:"steps"`
}

type Step struct {
	Do        string   `json:"do"`
	Target    Selector `json:"target"`
	Text      string   `json:"text,omitempty"`
	Key       string   `json:"key,omitempty"`
	Modifiers []string `json:"modifiers,omitempty"`
	Describe  string   `json:"describe,omitempty"`
	Final     bool     `json:"final,omitempty"`
	Fill      bool     `json:"fill,omitempty"`
}

type Selector struct {
	Role  string `json:"role"`
	Title string `json:"title,omitempty"`
	Index int    `json:"index,omitempty"`
}

var Verbs = map[string]bool{
	"click": true,
	"type":  true,
	"press": true,
}

// Virtual key codes, and the CGEventFlags that go with a held modifier. Both
// are stable parts of the platform ABI rather than values to look up at runtime.
var Keys = map[string]int{
	"Delete": 51,
	"Enter":  36,
	"Escape": 53,
	"Space":  49,
	"Tab":    48,
}

var Modifiers = map[string]uint64{
	"Command": 1 << 20,
	"Control": 1 << 18,
	"Option":  1 << 19,
	"Shift":   1 << 17,
}

// The names a step may use, for the schema the model answers against. Sorted, so
// a schema that is part of a cache key does not change from one run to the next.
func KeyNames() []string      { return namesOf(Keys) }
func ModifierNames() []string { return sortedNames(Modifiers) }

func namesOf(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sortedNames(m map[string]uint64) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// HeldDown is the combined flag mask for a step's modifiers.
func (s Step) HeldDown() uint64 {
	var flags uint64
	for _, name := range s.Modifiers {
		flags |= Modifiers[name]
	}
	return flags
}

// Chord names the key combination a step presses, the way a person would say it.
func (s Step) Chord() string {
	if len(s.Modifiers) == 0 {
		return s.Key
	}
	return strings.Join(append(append([]string{}, s.Modifiers...), s.Key), "-")
}

const MaxSteps = 8

func (p Program) Writes() bool { return len(p.Steps) > 0 }

func (p Program) Values() []string {
	var out []string
	for _, s := range p.Steps {
		if s.Text != "" {
			out = append(out, s.Text)
		}
		if s.Target.Title != "" {
			out = append(out, s.Target.Title)
		}
	}
	return out
}

func (p Program) Fill(replace func(string) string) Program {
	out := Program{App: p.App, Steps: make([]Step, len(p.Steps))}
	for i, s := range p.Steps {
		s.Text = replace(s.Text)
		s.Describe = replace(s.Describe)
		s.Target.Title = replace(s.Target.Title)
		out.Steps[i] = s
	}
	return out
}

func verb(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
