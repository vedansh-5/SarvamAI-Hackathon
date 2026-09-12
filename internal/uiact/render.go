package uiact

import (
	"encoding/json"
	"fmt"
	"strings"
)

func Render(p Program, t Tree) (string, error) {
	if err := Validate(p, t); err != nil {
		return "", err
	}

	app, err := literal(t.App)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, preamble, app)

	for _, s := range p.Steps {
		el, why := t.resolve(s.Target)
		if why != "" {
			return "", &InvalidError{App: t.App, Reasons: []string{why}}
		}

		path, err := json.Marshal(el.Path)
		if err != nil {
			return "", fmt.Errorf("uiact: encode path: %w", err)
		}
		roleLit, err := literal(el.Role)
		if err != nil {
			return "", err
		}
		titleLit, err := literal(el.Title)
		if err != nil {
			return "", err
		}

		switch verb(s.Do) {
		case "click":
			fmt.Fprintf(&b, "    click(%s, %s, %s);\n", path, roleLit, titleLit)
		case "type":
			text, err := literal(s.Text)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "    typeInto(%s, %s, %s, %s);\n", path, roleLit, titleLit, text)
		case "press":
			fmt.Fprintf(&b, "    press(%s, %s, %s, %d);\n", path, roleLit, titleLit, Keys[s.Key])
		}
	}

	b.WriteString(postamble)
	return b.String(), nil
}

const preamble = `(function () {
  const se = Application('System Events');
  const proc = se.applicationProcesses.byName(%s);
  if (proc.windows.length === 0) throw new Error('that application has no window open');
  const win = proc.windows[0];
  var lastTyped = null, lastText = '';
  function at(path) {
    var el = win;
    for (var i = 0; i < path.length; i++) { el = el.uiElements[path[i]]; }
    return el;
  }
  function label(el) {
    var t = '';
    try { t = String(el.title() || ''); } catch (e) {}
    if (!t) { try { t = String(el.description() || ''); } catch (e) {} }
    return t;
  }
  function check(path, role, title) {
    var el = at(path), r = '';
    try { r = String(el.role()); } catch (e) { throw new Error('the element is no longer there'); }
    if (r !== role) throw new Error('expected ' + role + ' and found ' + r);
    if (title && label(el) !== title) throw new Error('expected "' + title + '" and found "' + label(el) + '"');
    return el;
  }
  function click(path, role, title) {
    var el = check(path, role, title);
    el.actions.byName('AXPress').perform();
  }
  function typeInto(path, role, title, text) {
    var el = check(path, role, title);
    el.focused = true;
    el.value = text;
    lastTyped = el;
    lastText = text;
  }
  function press(path, role, title, code) {
    var el = check(path, role, title);
    el.focused = true;
    se.keyCode(code);
  }
  try {
`

const postamble = `    var verified = false;
    if (lastTyped) { try { verified = String(lastTyped.value()) === lastText; } catch (e) {} }
    return JSON.stringify({ ok: true, verified: verified });
  } catch (e) {
    return JSON.stringify({ ok: false, error: String((e && e.message) || e) });
  }
})();
`

func Preview(p Program) string {
	lines := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		if d := strings.TrimSpace(s.Describe); d != "" {
			lines = append(lines, d)
			continue
		}
		switch verb(s.Do) {
		case "click":
			lines = append(lines, "Click "+describe(s.Target))
		case "type":
			lines = append(lines, fmt.Sprintf("Type %q into %s", s.Text, describe(s.Target)))
		case "press":
			lines = append(lines, fmt.Sprintf("Press %s in %s", s.Key, describe(s.Target)))
		}
	}
	return strings.Join(lines, "\n")
}

func describe(sel Selector) string {
	switch {
	case sel.Title != "":
		return fmt.Sprintf("the %s %q", sel.Role, sel.Title)
	case sel.Index > 0:
		return fmt.Sprintf("%s number %d", sel.Role, sel.Index)
	default:
		return "the " + sel.Role
	}
}

func literal(v string) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("uiact: encode literal: %w", err)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
