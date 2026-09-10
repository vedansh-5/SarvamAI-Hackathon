package capability

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vedansh/vans/internal/capability/sdef"
)

type Surface struct {
	App   string
	Verbs []Verb
	Nouns []Noun
}

type Verb struct {
	Name     string
	Describe string
	Writes   bool
	Hidden   bool
	Direct   string
	Params   []Param
}

type Param struct {
	Name     string
	Type     string
	Optional bool
}

type Noun struct {
	Name       string
	Plural     string
	Describe   string
	Properties []Prop
	Contains   []string
}

type Prop struct {
	Name     string
	Type     string
	Writable bool
}

// The largest rendering handed to a model. Every application on the
// development machine fits several times over — the widest, Keynote, comes to
// roughly 2200 tokens — so this is a backstop against a pathological
// dictionary rather than a budget anything normally meets. It is a cold-path
// cost in any case: the program that comes out of it is cached, and the warm
// path spends nothing.
const MaxSurfaceBytes = 24000

// get and set appear in no dictionary on this machine, CocoaStandard.sdef
// included. They are part of the Apple Event Object Model rather than of any
// application's vocabulary, and every scriptable application answers them —
// which is also why JXA spells a property read as a call rather than as a
// command. A validator that knew only declared commands would reject every
// program that reads anything.
var universal = []Verb{
	{Name: "get", Describe: "read a property or element", Writes: false},
	{Name: "set", Describe: "change a property", Writes: true},
}

func Describe(app App, d *sdef.Dictionary) Surface {
	s := Surface{App: app.Name}
	s.Verbs = append(s.Verbs, universal...)

	for _, c := range allCommands(d) {
		if _, taken := s.Verb(c.Name); taken {
			continue
		}
		v := Verb{
			Name:     c.Name,
			Describe: c.Description,
			Writes:   c.AccessKind() == sdef.Write,
			Hidden:   c.Hidden == "yes",
		}
		if c.DirectParam != nil {
			v.Direct = typeOf(c.DirectParam.Type, c.DirectParam.TypeElems)
		}
		for _, p := range c.Params {
			if p.Hidden == "yes" {
				continue
			}
			v.Params = append(v.Params, Param{
				Name:     p.Name,
				Type:     typeOf(p.Type, p.TypeElems),
				Optional: p.Optional == "yes",
			})
		}
		s.Verbs = append(s.Verbs, v)
	}

	classes := merged(d)
	for _, c := range classes {
		n := Noun{Name: c.Name, Plural: c.Plural, Describe: c.Description}

		for _, p := range c.VisibleProperties() {
			n.Properties = append(n.Properties, Prop{
				Name:     p.Name,
				Type:     p.TypeName(),
				Writable: p.Writable(),
			})
		}

		for _, e := range c.Elements {
			if e.Hidden == "yes" || e.Type == "" {
				continue
			}
			n.Contains = append(n.Contains, e.Type)
		}

		s.Nouns = append(s.Nouns, n)
	}
	return s
}

// Hidden commands are collected too. Hidden means an application asked for it
// not to be listed in a dictionary browser; it does not mean the command is
// refused. Leaving them out would have the validator reject a program for
// using something that works.
func allCommands(d *sdef.Dictionary) []sdef.Command {
	var out []sdef.Command
	for _, s := range d.Suites {
		if s.Hidden == "yes" {
			continue
		}
		out = append(out, s.Commands...)
	}
	return out
}

func merged(d *sdef.Dictionary) []sdef.Class {
	byName := map[string]*sdef.Class{}
	var order []string

	for _, c := range d.Classes() {
		copied := c
		if existing, ok := byName[c.Name]; ok {
			existing.Properties = append(existing.Properties, copied.Properties...)
			existing.Elements = append(existing.Elements, copied.Elements...)
			continue
		}
		byName[c.Name] = &copied
		order = append(order, c.Name)
	}

	for _, s := range d.Suites {
		if s.Hidden == "yes" {
			continue
		}
		for _, ext := range s.ClassExtensions {
			if ext.Hidden == "yes" {
				continue
			}
			target, ok := byName[ext.Extends]
			if !ok {
				target = &sdef.Class{Name: ext.Extends, Description: ext.Description}
				byName[ext.Extends] = target
				order = append(order, ext.Extends)
			}
			target.Properties = append(target.Properties, ext.Properties...)
			target.Elements = append(target.Elements, ext.Elements...)
		}
	}

	out := make([]sdef.Class, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out
}

func typeOf(attr string, elems []sdef.TypeRef) string {
	if attr != "" {
		return attr
	}
	if len(elems) == 0 {
		return ""
	}
	t := elems[0].Type
	if elems[0].List == "yes" {
		return "list of " + t
	}
	return t
}

func (s Surface) Verb(name string) (Verb, bool) {
	for _, v := range s.Verbs {
		if strings.EqualFold(v.Name, name) {
			return v, true
		}
	}
	return Verb{}, false
}

func (s Surface) Writes(commands []string) bool {
	for _, name := range commands {
		if v, ok := s.Verb(name); ok && v.Writes {
			return true
		}
	}
	return false
}

func (s Surface) Unknown(commands []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, name := range commands {
		if seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		if _, ok := s.Verb(name); !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Text renders the whole surface. Commands are never dropped: they are what
// the validator checks a written program against, and a command missing from
// the rendering is one the model cannot use even though the application
// offers it.
func (s Surface) Text() string {
	return s.text(len(s.Nouns))
}

// TextWithin renders as much as fits, and says plainly how much did not.
// Objects go before properties do, so a trimmed surface still describes whole
// things rather than half-described ones.
func (s Surface) TextWithin(budget int) string {
	for n := len(s.Nouns); n > 0; n-- {
		if out := s.text(n); len(out) <= budget {
			return out
		}
	}
	return s.text(0)
}

func (s Surface) text(nouns int) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Application: %s\n\nCommands:\n", s.App)
	for _, v := range s.Verbs {
		if v.Hidden {
			continue
		}
		fmt.Fprintf(&b, "  %s", v.Name)
		if v.Direct != "" {
			fmt.Fprintf(&b, " <%s>", v.Direct)
		}
		for _, p := range v.Params {
			if p.Optional {
				fmt.Fprintf(&b, " [%s: %s]", p.Name, p.Type)
			} else {
				fmt.Fprintf(&b, " %s: %s", p.Name, p.Type)
			}
		}
		if v.Writes {
			b.WriteString("  (changes state)")
		}
		if v.Describe != "" {
			fmt.Fprintf(&b, "  — %s", v.Describe)
		}
		b.WriteByte('\n')
	}

	b.WriteString("\nObjects:\n")
	for _, n := range s.Nouns[:nouns] {
		fmt.Fprintf(&b, "  %s", n.Name)
		if n.Describe != "" {
			fmt.Fprintf(&b, " — %s", n.Describe)
		}
		b.WriteByte('\n')
		for _, p := range n.Properties {
			access := "read-only"
			if p.Writable {
				access = "writable"
			}
			fmt.Fprintf(&b, "    %s: %s (%s)\n", p.Name, p.Type, access)
		}
		if len(n.Contains) > 0 {
			fmt.Fprintf(&b, "    contains: %s\n", strings.Join(n.Contains, ", "))
		}
	}

	if left := len(s.Nouns) - nouns; left > 0 {
		fmt.Fprintf(&b,
			"\n%d further object types exist in this application and are not listed here.\n", left)
	}
	return b.String()
}
