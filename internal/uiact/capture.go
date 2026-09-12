package uiact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	MaxElements = 250
	MaxDepth    = 10
)

type Runner interface {
	Run(ctx context.Context, script string) ([]byte, error)
}

var ErrNoAssistiveAccess = errors.New("uiact: VANS has not been allowed to control this Mac through accessibility")

var ErrNoWindow = errors.New("uiact: that application is not showing a window to the accessibility API, which only lists windows for the application in front")

const assistiveFix = "Grant access in System Settings › Privacy & Security › Accessibility, then ask again."

func Fix(err error) string {
	if errors.Is(err, ErrNoAssistiveAccess) {
		return assistiveFix
	}
	return ""
}

func Capture(ctx context.Context, r Runner, app string) (Tree, error) {
	name, err := literal(app)
	if err != nil {
		return Tree{}, err
	}

	out, err := r.Run(ctx, fmt.Sprintf(captureScript, name, MaxElements, MaxDepth))
	if err != nil {
		return Tree{}, classify(err.Error(), err)
	}

	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Value Tree   `json:"value"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return Tree{}, fmt.Errorf("uiact: unreadable reply (output %q): %w", truncate(string(out), 200), err)
	}
	if !envelope.OK {
		return Tree{}, classify(envelope.Error, fmt.Errorf("uiact: %s", envelope.Error))
	}
	return envelope.Value, nil
}

func classify(msg string, fallback error) error {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "assistive access"):
		return ErrNoAssistiveAccess
	case strings.Contains(lower, "no window open"):
		return ErrNoWindow
	default:
		return fallback
	}
}

const captureScript = `(function () {
  const se = Application('System Events');
  function reply(f) {
    try { return JSON.stringify({ ok: true, value: f() }); }
    catch (e) { return JSON.stringify({ ok: false, error: String((e && e.message) || e) }); }
  }
  return reply(function () {
    const proc = se.applicationProcesses.byName(%s);
    if (proc.windows.length === 0) throw new Error('that application has no window open');
    const win = proc.windows[0];
    const limit = %d, maxDepth = %d;
    const leaf = {
      AXStaticText: true, AXButton: true, AXImage: true, AXMenuButton: true,
      AXCheckBox: true, AXRadioButton: true, AXSlider: true, AXProgressIndicator: true,
      AXValueIndicator: true, AXPopUpButton: true
    };
    const out = [];
    function walk(el, path, depth) {
      if (out.length >= limit || depth > maxDepth) return;
      let kids, roles, n = 0;
      try { kids = el.uiElements; roles = kids.role(); n = roles.length; } catch (e) { return; }
      if (!n) return;
      let titles = [], descs = [], values = [], enabled = [], focused = [];
      try { titles = kids.title(); } catch (e) {}
      try { descs = kids.description(); } catch (e) {}
      try { values = kids.value(); } catch (e) {}
      try { enabled = kids.enabled(); } catch (e) {}
      try { focused = kids.focused(); } catch (e) {}
      const mine = [];
      for (let i = 0; i < n && out.length < limit; i++) {
        try {
          const r = String(roles[i] || '');
          let title = titles[i] ? String(titles[i]) : '';
          if (!title && descs[i]) title = String(descs[i]);
          const v = values[i];
          const value = (v === null || v === undefined) ? '' : String(v);
          out.push({
            role: r, title: title, value: value.slice(0, 120),
            enabled: enabled[i] !== false, focused: focused[i] === true,
            actions: [], path: path.concat([i])
          });
          mine.push([i, r]);
        } catch (e) {}
      }
      for (let j = 0; j < mine.length && out.length < limit; j++) {
        if (leaf[mine[j][1]]) continue;
        try { walk(kids[mine[j][0]], path.concat([mine[j][0]]), depth + 1); } catch (e) {}
      }
    }
    try { walk(win, [], 0); } catch (e) {}
    let title = '';
    try { title = String(win.title() || ''); } catch (e) {}
    return { app: String(proc.name()), window: title, elements: out };
  });
})();
`

func Frontmost(ctx context.Context, r Runner) (string, error) {
	out, err := r.Run(ctx, frontmostScript)
	if err != nil {
		return "", classify(err.Error(), err)
	}

	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return "", fmt.Errorf("uiact: unreadable reply (output %q): %w", truncate(string(out), 200), err)
	}
	if !envelope.OK {
		return "", classify(envelope.Error, fmt.Errorf("uiact: %s", envelope.Error))
	}
	return envelope.Value, nil
}

const frontmostScript = `(function () {
  const se = Application('System Events');
  function reply(f) {
    try { return JSON.stringify({ ok: true, value: f() }); }
    catch (e) { return JSON.stringify({ ok: false, error: String((e && e.message) || e) }); }
  }
  return reply(function () {
    const front = se.applicationProcesses.whose({ frontmost: true })();
    if (front.length === 0) throw new Error('no application is in front');
    return String(front[0].name());
  });
})();
`
