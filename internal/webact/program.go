// Package webact acts inside a page: clicking, typing, submitting.
//
// It exists because most of what people want to drive has no scripting
// dictionary. Telegram, Notion, Discord and Slack ship no .sdef, so the
// application tier (D-042) cannot reach them — but every one of them has a
// web version that the browser already drives. Going through the page is the
// cheap path: no new permission, no new dependency, and the same primitive as
// everything else.
//
// The program is data rather than code, for the same reason it is in appact:
// a model handed a page and asked for JavaScript will produce something whose
// effect can only be established by reading it. Here the steps are checked
// against the DOM that was actually captured, before anything runs.
package webact

import "strings"

// Program is a sequence of steps against one page.
type Program struct {
	URL   string `json:"url"`
	Steps []Step `json:"steps"`
}

// Step is one thing done to one element.
type Step struct {
	// Do is click, type, or press.
	Do string `json:"do"`

	// Selector addresses the element. CSS, checked against the page.
	Selector string `json:"selector"`

	// Text is what to type. Ignored by the other verbs.
	Text string `json:"text,omitempty"`

	// Key is what to press: Enter, Escape, Tab. Ignored by the others.
	Key string `json:"key,omitempty"`

	// Describe is what this step does, in the page's own words, for the
	// confirmation. The model writes it; it is shown, never executed.
	Describe string `json:"describe,omitempty"`
}

// Verbs are everything a program may do.
//
// Deliberately short. Every addition is a new way for a cached program to do
// something its preview did not describe, and clicking and typing is what
// using a page is made of.
var Verbs = map[string]bool{
	"click": true,
	"type":  true,
	"press": true,
}

// Keys are the keys a program may press. A page cannot be driven by arbitrary
// key events without becoming a way to send anything anywhere.
var Keys = map[string]bool{
	"Enter":  true,
	"Escape": true,
	"Tab":    true,
}

// Writes reports whether running this program changes anything.
//
// Always true, and that is not laziness. The application tier asks the
// dictionary whether a command mutates; a page declares nothing, so there is
// nothing to ask. Clicking is indistinguishable from sending. Every web
// program is confirmed.
func (p Program) Writes() bool { return len(p.Steps) > 0 }

// Values lists the literal text a program carries, for the leak check.
func (p Program) Values() []string {
	var out []string
	for _, s := range p.Steps {
		if s.Text != "" {
			out = append(out, s.Text)
		}
	}
	return out
}

// Fill replaces placeholders in the text a program types.
//
// Selectors are not filled. They came from the page's structure, which does
// not change between requests of the same shape; text does, and is the only
// thing that should.
func (p Program) Fill(replace func(string) string) Program {
	out := Program{URL: p.URL, Steps: make([]Step, len(p.Steps))}
	for i, s := range p.Steps {
		s.Text = replace(s.Text)
		s.Describe = replace(s.Describe)
		out.Steps[i] = s
	}
	return out
}

func verb(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
