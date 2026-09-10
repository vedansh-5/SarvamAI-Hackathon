package appact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/capability"
)

// Runner executes a script and returns its trimmed stdout. It is the same
// interface appleevents.Osascript satisfies.
type Runner interface {
	Run(ctx context.Context, script string) ([]byte, error)
}

// Receipt is what happened, read back from the application rather than
// asserted by the runtime.
//
// Verified is the D-004 distinction: a state change is only confirmed when the
// application will say the object is there. A command that returned without
// error and an object that exists are not the same claim, and the window says
// which one it has.
type Receipt struct {
	ID       string
	Name     string
	Value    string
	Verified bool
}

// Run renders a program, executes it, and reads the result back.
func Run(ctx context.Context, r Runner, p Program, s capability.Surface) (Receipt, error) {
	script, err := Render(p, s)
	if err != nil {
		return Receipt{}, err
	}

	out, err := r.Run(ctx, script)
	if err != nil {
		return Receipt{}, fmt.Errorf("appact: %s refused the program: %w", s.App, err)
	}

	var result struct {
		OK    bool   `json:"ok"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return Receipt{}, fmt.Errorf("appact: could not read back what %s did: %w (output %q)",
			s.App, err, out)
	}

	receipt := Receipt{ID: result.ID, Name: result.Name, Value: result.Value}

	// An id means the application handed back a real object and it was still
	// addressable when asked. Without one there is nothing to verify against,
	// which is a weaker claim and is reported as such rather than rounded up.
	receipt.Verified = result.OK && result.ID != ""
	return receipt, nil
}

// Preview is what the confirmation shows: the steps in the application's own
// words, not the script.
//
// The script is exact and unreadable, and a preview nobody reads is a
// confirmation nobody gives. This says what will happen in the vocabulary the
// user would use to describe it.
func Preview(p Program, s capability.Surface) string {
	var out string
	for i, step := range p.Steps {
		if i > 0 {
			out += "\n"
		}
		out += describeStep(step, s)
	}
	return out
}

func describeStep(step Step, s capability.Surface) string {
	where := ""
	if step.Target != nil {
		where = describeSelector(*step.Target)
	}

	switch lower(step.Command) {
	case "make":
		line := fmt.Sprintf("Create a %s in %s", step.Make, s.App)
		if where != "" {
			line = fmt.Sprintf("Create a %s in %s", step.Make, where)
		}
		return line + describeProperties(step.With)

	case "set":
		for name, value := range step.With {
			return fmt.Sprintf("Set the %s of %s to %q", name, orApp(where, s.App), value)
		}
		return "Change " + orApp(where, s.App)

	case "get":
		return "Read " + orApp(where, s.App)

	case "delete":
		return "Delete " + orApp(where, s.App)

	default:
		return fmt.Sprintf("%s %s", capitalise(step.Command), orApp(where, s.App))
	}
}

func describeSelector(sel Selector) string {
	switch {
	case sel.Name != "":
		return fmt.Sprintf("the %s named %q", sel.Class, sel.Name)
	case sel.Index > 0:
		return fmt.Sprintf("%s number %d", sel.Class, sel.Index)
	default:
		return "every " + sel.Class
	}
}

func describeProperties(with map[string]string) string {
	if len(with) == 0 {
		return ""
	}
	out := ", with"
	first := true
	for name, value := range with {
		if !first {
			out += " and"
		}
		first = false
		out += fmt.Sprintf(" %s %q", name, value)
	}
	return out
}

func orApp(where, app string) string {
	if where == "" {
		return app
	}
	return where
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func lower(s string) string { return strings.ToLower(s) }
