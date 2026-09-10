package appact

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/capability"
)

// Render turns a validated program into the script that runs it.
//
// Every value from the model reaches the script through encoding/json, which
// escapes quotes, backslashes, control characters and U+2028/U+2029 — so a
// property value cannot close its own literal and become code. Identifiers do
// not go through it: they are checked against the dictionary first, and only
// a name the application declared is ever written out.
func Render(p Program, s capability.Surface) (string, error) {
	if err := Validate(p, s); err != nil {
		return "", err
	}

	var b strings.Builder
	app, err := literal(s.App)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "const app = Application(%s);\n", app)

	plurals := map[string]string{}
	for _, n := range s.Nouns {
		plurals[strings.ToLower(n.Name)] = n.Plural
	}

	last := "null"
	for i, step := range p.Steps {
		expr, err := stepExpr(step, plurals)
		if err != nil {
			return "", err
		}
		if step.Bind != "" {
			if !identifier(step.Bind) {
				return "", fmt.Errorf("appact: %q is not usable as a name", step.Bind)
			}
			fmt.Fprintf(&b, "const %s = %s;\n", step.Bind, expr)
			last = step.Bind
			continue
		}
		name := fmt.Sprintf("_%d", i)
		fmt.Fprintf(&b, "const %s = %s;\n", name, expr)
		last = name
	}

	fmt.Fprintf(&b, "JSON.stringify(describe(%s));\n", last)
	return preamble + b.String(), nil
}

// describe reads a result back without assuming what it is. A make returns an
// object specifier, a get can return anything, and a delete returns nothing —
// the receipt has to survive all three.
const preamble = `function describe(v) {
  if (v === null || v === undefined) return { ok: true };
  try {
    if (typeof v.id === 'function') {
      return { ok: true, id: String(v.id()), name: String(v.name()) };
    }
  } catch (e) {}
  try {
    return { ok: true, value: String(typeof v === 'function' ? v() : v) };
  } catch (e) {
    return { ok: true };
  }
}
`

func stepExpr(step Step, plurals map[string]string) (string, error) {
	target := "app"
	if step.Target != nil {
		t, err := selectorExpr(*step.Target, plurals)
		if err != nil {
			return "", err
		}
		target = t
	}

	switch strings.ToLower(step.Command) {
	case "make":
		props, err := properties(step.With)
		if err != nil {
			return "", err
		}
		class, err := literal(step.Make)
		if err != nil {
			return "", err
		}
		if step.Target != nil {
			return fmt.Sprintf("app.make({ new: %s, at: %s, withProperties: %s })",
				class, target, props), nil
		}
		return fmt.Sprintf("app.make({ new: %s, withProperties: %s })", class, props), nil

	case "set":
		if len(step.With) != 1 {
			return "", fmt.Errorf("appact: set changes exactly one property at a time")
		}
		for name, value := range step.With {
			if !identifier(property(name)) {
				return "", fmt.Errorf("appact: %q is not usable as a property", name)
			}
			v, err := literal(value)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("(%s.%s = %s)", target, property(name), v), nil
		}
		return "", fmt.Errorf("appact: set with nothing to set")

	case "get":
		// Deliberately not evaluated here. In JXA a specifier is a function,
		// and calling it turns a live reference into a dead copy — so a step
		// that binds a list and a later step that puts something in it would
		// be addressing an object the application no longer recognises.
		// Evaluation happens once, at the end, on the result being reported.
		return target, nil

	default:
		if !identifier(step.Command) {
			return "", fmt.Errorf("appact: %q is not usable as a command", step.Command)
		}
		if step.Target != nil {
			return fmt.Sprintf("app.%s(%s)", step.Command, target), nil
		}
		return fmt.Sprintf("app.%s()", step.Command), nil
	}
}

func selectorExpr(sel Selector, plurals map[string]string) (string, error) {
	root := "app"
	if sel.In != "" {
		if !identifier(sel.In) {
			return "", fmt.Errorf("appact: %q is not usable as a name", sel.In)
		}
		root = sel.In
	}

	collection := plurals[strings.ToLower(sel.Class)]
	if collection == "" {
		collection = Plural(sel.Class)
	}
	collection = property(collection)
	if !identifier(collection) {
		return "", fmt.Errorf("appact: %q has no usable collection name", sel.Class)
	}

	switch {
	case sel.Name != "":
		name, err := literal(sel.Name)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.%s.byName(%s)", root, collection, name), nil
	case sel.Index > 0:
		return fmt.Sprintf("%s.%s.at(%d)", root, collection, sel.Index-1), nil
	default:
		return fmt.Sprintf("%s.%s", root, collection), nil
	}
}

func properties(with map[string]string) (string, error) {
	if len(with) == 0 {
		return "{}", nil
	}
	pairs := make([]string, 0, len(with))
	for name, value := range with {
		key := property(name)
		if !identifier(key) {
			return "", fmt.Errorf("appact: %q is not usable as a property", name)
		}
		v, err := literal(value)
		if err != nil {
			return "", err
		}
		pairs = append(pairs, fmt.Sprintf("%s: %s", key, v))
	}
	return "{ " + strings.Join(pairs, ", ") + " }", nil
}

// property turns a dictionary's spelling into JXA's. Dictionaries write
// multi-word names with spaces — "file transfer", "creation date" — and the
// scripting bridge exposes them camel-cased.
func property(name string) string {
	parts := strings.Fields(name)
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

// Plural derives a collection name for a class that did not declare one.
//
// Ten of the dictionaries on this machine declare plural on the class and
// those are used verbatim. The rest, Notes among them, leave the scripting
// bridge to apply ordinary English, which is what this is.
func Plural(class string) string {
	words := strings.Fields(class)
	if len(words) == 0 {
		return ""
	}
	last := words[len(words)-1]

	switch {
	case strings.HasSuffix(last, "y") && len(last) > 1 && !isVowel(last[len(last)-2]):
		last = last[:len(last)-1] + "ies"
	case strings.HasSuffix(last, "s"), strings.HasSuffix(last, "x"),
		strings.HasSuffix(last, "ch"), strings.HasSuffix(last, "sh"):
		last += "es"
	default:
		last += "s"
	}

	words[len(words)-1] = last
	return strings.Join(words, " ")
}

func isVowel(c byte) bool { return strings.IndexByte("aeiou", c) >= 0 }

// literal encodes a value as a JavaScript string.
//
// HTML escaping is off. It is on by default, and it is not doing any work
// here — <, > and & are ordinary characters inside a JS string — while it
// turns a note body into \u003cdiv\u003e in a script the user is shown before
// confirming. What still gets escaped is what matters: quotes, backslashes,
// control characters, and U+2028/U+2029, which JavaScript treats as line
// terminators even though JSON does not.
func literal(v string) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("appact: encode literal: %w", err)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// identifier is the last line before a name is written into a script.
//
// Nothing reaching here should be able to fail it — names come from the
// dictionary and are checked against it first — which is exactly why it is
// here. It is the check that holds if one of the earlier ones is wrong.
func identifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == '$':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
