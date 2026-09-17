package webstyle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

func encode(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("webstyle: encode: %w", err)
	}
	return strings.TrimSpace(b.String()), nil
}

// The two palettes a restyle can wear.
//
// Every colour is a tint with alpha rather than a solid, so it takes on
// whatever it is laid over instead of fighting it, and every ring is the same
// hue as its tint so the mark reads as one thing. The dark set is not the light
// set dimmed: tints go down so they do not glow, and rings go up, because a
// hairline that works on white disappears on near-black.
const lightTokens = `:root{` +
	`--vans-tint:rgba(250,204,21,.28);--vans-ring:rgba(161,98,7,.32);` +
	`--vans-rose:rgba(244,63,94,.18);--vans-rose-ring:rgba(159,18,57,.30);` +
	`--vans-blue:rgba(59,130,246,.18);--vans-blue-ring:rgba(29,78,216,.30)}`

const darkTokens = `:root{` +
	`--vans-tint:rgba(250,204,21,.20);--vans-ring:rgba(250,204,21,.38);` +
	`--vans-rose:rgba(251,113,133,.20);--vans-rose-ring:rgba(251,113,133,.38);` +
	`--vans-blue:rgba(96,165,250,.20);--vans-blue-ring:rgba(96,165,250,.38)}`

// Render turns a validated program into the script that applies it.
//
// The palette is chosen on the page rather than here, because only the page
// knows what it is wearing: the script reads the background it will actually
// sit on, walking up from the body until it finds a colour that is not
// transparent, and picks the set that will be legible against it.
func Render(p Program, doc *html.Node) (string, error) {
	if err := Validate(p, doc); err != nil {
		return "", err
	}

	css, err := encode(Sheet(p))
	if err != nil {
		return "", err
	}
	light, err := encode(lightTokens)
	if err != nil {
		return "", err
	}
	dark, err := encode(darkTokens)
	if err != nil {
		return "", err
	}

	selectors := make([]string, 0, len(p.Rules))
	for _, r := range p.Rules {
		selectors = append(selectors, strings.TrimSpace(r.Selector))
	}
	list, err := encode(selectors)
	if err != nil {
		return "", err
	}

	id, err := encode(StyleElementID)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(applyScript, id, css, list, light, dark), nil
}

const applyScript = `(function () {
  try {
    const id = %[1]s;

    function channels(value) {
      const m = String(value || '').match(/[\d.]+/g);
      if (!m || m.length < 3) return null;
      const a = m.length > 3 ? parseFloat(m[3]) : 1;
      if (a < 0.2) return null;
      return [parseFloat(m[0]), parseFloat(m[1]), parseFloat(m[2])];
    }

    function pageIsDark() {
      let node = document.body || document.documentElement;
      while (node) {
        const rgb = channels(getComputedStyle(node).backgroundColor);
        if (rgb) {
          return (0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2]) < 128;
        }
        node = node.parentElement;
      }
      return false;
    }

    let el = document.getElementById(id);
    if (!el) {
      el = document.createElement('style');
      el.id = id;
      document.documentElement.appendChild(el);
    }
    const tokens = pageIsDark() ? %[5]s : %[4]s;
    el.textContent = tokens + '\n' + %[2]s;

    let matched = 0;
    for (const sel of %[3]s) {
      try { matched += document.querySelectorAll(sel).length; } catch (e) {}
    }
    return JSON.stringify({ ok: true, matched: matched, dark: pageIsDark() });
  } catch (e) {
    return JSON.stringify({ ok: false, error: String((e && e.message) || e) });
  }
})();
`

// Clear removes a restyle, leaving the page as it was.
func Clear() string {
	id, err := encode(StyleElementID)
	if err != nil {
		id = `"vans-style"`
	}
	return fmt.Sprintf(clearScript, id)
}

const clearScript = `(function () {
  try {
    const el = document.getElementById(%[1]s);
    if (el && el.parentNode) el.parentNode.removeChild(el);
    return JSON.stringify({ ok: true, matched: 0 });
  } catch (e) {
    return JSON.stringify({ ok: false, error: String((e && e.message) || e) });
  }
})();
`
