package webact

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// Render turns a validated program into the script that runs it.
//
// Selectors and text reach the script through encoding/json, so neither can
// close its own literal. Nothing else from the model is written out: the verbs
// are matched against a fixed set and rendered as code here.
func Render(p Program, doc *html.Node) (string, error) {
	if err := Validate(p, doc); err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString(preamble)

	for _, s := range p.Steps {
		sel, err := literal(s.Selector)
		if err != nil {
			return "", err
		}

		switch verb(s.Do) {
		case "click":
			fmt.Fprintf(&b, "  click(%s);\n", sel)
		case "type":
			text, err := literal(s.Text)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "  typeInto(%s, %s);\n", sel, text)
		case "press":
			key, err := literal(s.Key)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "  press(%s, %s);\n", sel, key)
		}
	}

	b.WriteString(postamble)
	return b.String(), nil
}

// The page is re-checked at run time as well as at validation time. The DOM
// was captured a moment ago and a live page moves: an element that was there
// when the plan was made can be gone when it is confirmed, and clicking
// nothing while reporting success is the failure worth preventing.
//
// Events are dispatched the way a browser does rather than by setting .value,
// because every framework worth naming listens for input and change and
// ignores a value that arrives without them — the box shows the text, the app
// never sees it, and the message sends empty.
const preamble = `(function () {
  function one(sel) {
    const found = document.querySelectorAll(sel);
    if (found.length === 0) throw new Error('nothing matches ' + sel + ' any more');
    if (found.length > 1) throw new Error(found.length + ' elements match ' + sel);
    return found[0];
  }
  function click(sel) {
    const el = one(sel);
    el.scrollIntoView({ block: 'center' });
    el.click();
  }
  function typeInto(sel, text) {
    const el = one(sel);
    el.focus();
    if (el.isContentEditable) {
      el.textContent = text;
    } else {
      const proto = el instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
      const setter = Object.getOwnPropertyDescriptor(proto, 'value').set;
      setter.call(el, text);
    }
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }
  function press(sel, key) {
    const el = one(sel);
    el.focus();
    for (const type of ['keydown', 'keypress', 'keyup']) {
      el.dispatchEvent(new KeyboardEvent(type, {
        key: key, code: key, bubbles: true, cancelable: true,
      }));
    }
  }
  try {
`

const postamble = `    return JSON.stringify({ ok: true });
  } catch (e) {
    return JSON.stringify({ ok: false, error: String((e && e.message) || e) });
  }
})();
`

// Preview is what the confirmation shows: the steps in the page's own words.
func Preview(p Program) string {
	lines := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		if d := strings.TrimSpace(s.Describe); d != "" {
			lines = append(lines, d)
			continue
		}
		switch verb(s.Do) {
		case "click":
			lines = append(lines, "Click "+s.Selector)
		case "type":
			lines = append(lines, fmt.Sprintf("Type %q into %s", s.Text, s.Selector))
		case "press":
			lines = append(lines, fmt.Sprintf("Press %s in %s", s.Key, s.Selector))
		}
	}
	return strings.Join(lines, "\n")
}

func literal(v string) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("webact: encode literal: %w", err)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// Parse is the captured page, ready to validate against.
func Parse(src string) (*html.Node, error) {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("webact: parse page: %w", err)
	}
	return doc, nil
}
