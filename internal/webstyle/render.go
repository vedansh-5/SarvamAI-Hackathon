package webstyle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"github.com/vedansh/vans/internal/pagemark"
	"github.com/vedansh/vans/internal/theme"
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

func lightTokens(t theme.Theme) string {
	return `:root{` +
		`--vans-tint:` + t.Soft + `;--vans-ring:` + theme.Alpha(t.Accent, .55) + `;` +
		`--vans-rose:rgba(244,63,94,.18);--vans-rose-ring:rgba(159,18,57,.30);` +
		`--vans-blue:rgba(59,130,246,.18);--vans-blue-ring:rgba(29,78,216,.30)}`
}

func darkTokens(t theme.Theme) string {
	return `:root{` +
		`--vans-tint:` + theme.Alpha(t.Accent, .24) + `;--vans-ring:` + theme.Alpha(t.Accent, .55) + `;` +
		`--vans-rose:rgba(251,113,133,.20);--vans-rose-ring:rgba(251,113,133,.38);` +
		`--vans-blue:rgba(96,165,250,.20);--vans-blue-ring:rgba(96,165,250,.38)}`
}

// Render turns a validated program into the script that applies it.
//
// The palette is chosen on the page rather than here, because only the page
// knows what it is wearing: the script reads the background it will actually
// sit on, walking up from the body until it finds a colour that is not
// transparent, and picks the set that will be legible against it.
func Render(p Program, doc *html.Node, t theme.Theme) (string, error) {
	if err := Validate(p, doc); err != nil {
		return "", err
	}

	css, err := encode(Sheet(p))
	if err != nil {
		return "", err
	}
	light, err := encode(lightTokens(t))
	if err != nil {
		return "", err
	}
	dark, err := encode(darkTokens(t))
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

    const old = document.getElementById(id);
    if (old && old.parentNode) old.parentNode.removeChild(old);
    if (window.__vansStyle) {
      const was = window.__vansStyle;
      document.adoptedStyleSheets = document.adoptedStyleSheets.filter((x) => x !== was);
    }
    const tokens = pageIsDark() ? %[5]s : %[4]s;
    const sheet = new CSSStyleSheet();
    sheet.replaceSync(tokens + '\n' + %[2]s);
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
    window.__vansStyle = sheet;

    let matched = 0;
    for (const sel of %[3]s) {
      try { matched += document.querySelectorAll(sel).length; } catch (e) {}
    }
    if (sheet.cssRules.length === 0) return JSON.stringify({ ok: false, error: 'the page would not take the highlighting' });
    return JSON.stringify({ ok: true, matched: matched, dark: pageIsDark() });
  } catch (e) {
    return JSON.stringify({ ok: false, error: String((e && e.message) || e) });
  }
})();
`

// Clear removes a restyle, leaving the page as it was.
//
// It reports whether it found one to remove, which is what lets a caller sweep
// every tab and still say how many actually carried a restyle. Running it on a
// page that was never restyled is not a failure — it is the answer "nothing
// here" — so it succeeds either way.
func Clear() string {
	id, err := encode(StyleElementID)
	if err != nil {
		id = `"vans-style"`
	}
	return fmt.Sprintf(clearScript, id, pagemark.ClearJS)
}

const clearScript = `(function () {
  try {
    let removed = false;
    const el = document.getElementById(%[1]s);
    if (el && el.parentNode) { el.parentNode.removeChild(el); removed = true; }
    if (window.__vansStyle) {
      const was = window.__vansStyle;
      document.adoptedStyleSheets = document.adoptedStyleSheets.filter((x) => x !== was);
      delete window.__vansStyle;
      removed = true;
    }
    %[2]s
    return JSON.stringify({ ok: true, matched: 0, removed: removed });
  } catch (e) {
    return JSON.stringify({ ok: false, error: String((e && e.message) || e) });
  }
})();
`
