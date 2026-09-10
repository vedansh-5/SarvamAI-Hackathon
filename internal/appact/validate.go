package appact

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vedansh/vans/internal/capability"
)

// InvalidError is a program that cannot be run, with every reason at once.
//
// Every reason rather than the first, because these go back to the model on
// rung 2 of the ladder (D-020), and a model told about one problem at a time
// fixes one problem at a time.
type InvalidError struct {
	App     string
	Reasons []string
}

func (e *InvalidError) Error() string {
	return fmt.Sprintf("appact: this program will not run against %s: %s",
		e.App, strings.Join(e.Reasons, "; "))
}

// Validate checks a program against what the application actually declares.
//
// These are floors, not preferences. A model cannot argue its way past any of
// them, because none of them consult it: they consult the dictionary the
// application shipped.
func Validate(p Program, s capability.Surface) error {
	var reasons []string
	add := func(format string, args ...any) {
		reasons = append(reasons, fmt.Sprintf(format, args...))
	}

	if !strings.EqualFold(p.App, s.App) {
		add("it addresses %q but was written for %q", p.App, s.App)
	}
	if len(p.Steps) == 0 {
		add("it has no steps")
	}

	for _, name := range s.Unknown(p.Commands()) {
		add("%s has no %q command", s.App, name)
	}

	nouns := map[string]capability.Noun{}
	for _, n := range s.Nouns {
		nouns[strings.ToLower(n.Name)] = n
	}
	for _, class := range p.Classes() {
		if _, ok := nouns[strings.ToLower(class)]; !ok {
			add("%s has no %q object", s.App, class)
		}
	}

	bound := map[string]bool{}
	for i, step := range p.Steps {
		where := fmt.Sprintf("step %d", i+1)

		if step.Target != nil && step.Target.In != "" && !bound[step.Target.In] {
			add("%s looks inside %q, which no earlier step produced", where, step.Target.In)
		}
		if step.Bind != "" {
			if bound[step.Bind] {
				add("%s binds %q, which is already taken", where, step.Bind)
			}
			bound[step.Bind] = true
		}

		if strings.EqualFold(step.Command, "make") && step.Make == "" {
			add("%s makes something without saying what", where)
		}
		if !strings.EqualFold(step.Command, "make") && step.Make != "" {
			add("%s names a class to create but its command is %q", where, step.Command)
		}

		reasons = append(reasons, badProperties(where, step, nouns)...)
	}

	if len(reasons) == 0 {
		return nil
	}
	sort.Strings(reasons)
	return &InvalidError{App: s.App, Reasons: reasons}
}

// badProperties rejects properties the class does not have, and properties it
// has but will not accept a change to.
//
// The read-only case is the one worth having. It fails at the Apple event
// rather than at the parser, which means without this it fails *after* the
// user has confirmed a plan that said it would work.
func badProperties(where string, step Step, nouns map[string]capability.Noun) []string {
	if len(step.With) == 0 {
		return nil
	}

	class := step.Make
	if class == "" && step.Target != nil {
		class = step.Target.Class
	}
	if class == "" {
		return []string{fmt.Sprintf("%s sets properties without naming what it is acting on", where)}
	}

	noun, ok := nouns[strings.ToLower(class)]
	if !ok {
		return nil // already reported as an unknown class
	}

	byName := map[string]capability.Prop{}
	for _, p := range noun.Properties {
		byName[strings.ToLower(p.Name)] = p
	}

	var out []string
	for name := range step.With {
		p, ok := byName[strings.ToLower(name)]
		if !ok {
			out = append(out, fmt.Sprintf("%s: a %s has no %q", where, class, name))
			continue
		}
		if !p.Writable {
			out = append(out, fmt.Sprintf("%s: a %s's %q cannot be changed", where, class, name))
		}
	}
	return out
}

// Writes reports whether running this program changes anything.
//
// Asked of the dictionary, never of the model. Whether a confirmation appears
// is the one decision on the screen with consequences, and a model that could
// label its own write a read would be making it.
func Writes(p Program, s capability.Surface) bool {
	if s.Writes(p.Commands()) {
		return true
	}
	for _, step := range p.Steps {
		if len(step.With) > 0 {
			return true
		}
	}
	return false
}

// Baked reports values the program hardcoded instead of referencing.
//
// The same floor as D-039, one tier down and for the same reason: the program
// is cached under a shape and replayed for every instruction of that shape, so
// a value written into it is written into every future use. It is silent —
// the note is created, nothing errors, and the text is the first note's text
// forever.
func Baked(p Program, literals map[string]string) []string {
	var out []string
	seen := map[string]bool{}

	for _, value := range p.Values() {
		for _, literal := range literals {
			literal = strings.TrimSpace(literal)
			// Short values occur inside longer ones and inside ordinary
			// words, the same exemption the routing floor makes.
			if len(literal) < 3 || seen[literal] {
				continue
			}
			if strings.Contains(strings.ToLower(value), strings.ToLower(literal)) {
				seen[literal] = true
				out = append(out, literal)
			}
		}
	}
	sort.Strings(out)
	return out
}
