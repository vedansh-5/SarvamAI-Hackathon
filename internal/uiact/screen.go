package uiact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const StepPause = "0.35"

func RenderScreen(p Program, t Tree) (string, error) {
	if err := Validate(p, t); err != nil {
		return "", err
	}

	app, err := literal(t.App)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, screenPreamble, app, StepPause)

	for i, s := range p.Steps {
		el, why := t.resolve(s.Target)
		if why != "" {
			return "", &InvalidError{App: t.App, Reasons: []string{why}}
		}

		switch verb(s.Do) {
		case "click", "type":
			x, y, ok := el.Center()
			if !ok {
				return "", &InvalidError{App: t.App, Reasons: []string{fmt.Sprintf(
					"step %d: the %s %q reports no position on screen, so it cannot be clicked",
					i+1, el.Role, el.Title)}}
			}
			fmt.Fprintf(&b, "    clickAt(%d, %d);\n", x, y)

			if verb(s.Do) == "type" {
				text, err := literal(s.Text)
				if err != nil {
					return "", err
				}
				fmt.Fprintf(&b, "    typeText(%s);\n", text)
			}

		case "press":
			fmt.Fprintf(&b, "    pressKey(%d);\n", Keys[s.Key])
		}
	}

	b.WriteString(screenPostamble)
	return b.String(), nil
}

const screenPreamble = `(function () {
  const se = Application('System Events');
  const sa = Application.currentApplication();
  sa.includeStandardAdditions = true;
  const pause = %[2]s;
  function bringForward() {
    const proc = se.applicationProcesses.byName(%[1]s);
    if (!proc.frontmost()) {
      proc.frontmost = true;
      sa.delay(0.4);
    }
  }
  function clickAt(x, y) {
    se.click({ at: [x, y] });
    sa.delay(pause);
  }
  function typeText(text) {
    se.keystroke(text);
    sa.delay(pause);
  }
  function pressKey(code) {
    se.keyCode(code);
    sa.delay(pause);
  }
  try {
    bringForward();
`

const screenPostamble = `    return JSON.stringify({ ok: true });
  } catch (e) {
    return JSON.stringify({ ok: false, error: String((e && e.message) || e) });
  }
})();
`

func RunScreen(ctx context.Context, r Runner, p Program, t Tree) error {
	script, err := RenderScreen(p, t)
	if err != nil {
		return err
	}

	pid, err := pidFor(ctx, r, t.App)
	if err != nil {
		return err
	}
	if err := bringForward(ctx, r, t.App); err != nil {
		return err
	}
	if err := VerifyTargets(pid, p, t); err != nil {
		return err
	}

	out, err := r.Run(ctx, script)
	if err != nil {
		return classify(err.Error(), fmt.Errorf("uiact: %s refused the step: %w", t.App, err))
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return fmt.Errorf("uiact: could not read back what happened (output %q): %w",
			truncate(string(out), 200), err)
	}
	if !result.OK {
		return classify(result.Error, fmt.Errorf("uiact: the step did not go through: %s", result.Error))
	}
	return nil
}

var lookupAt = elementAt

func VerifyTargets(pid int, p Program, t Tree) error {
	for i, step := range p.Steps {
		if verb(step.Do) == "press" {
			continue
		}

		want, why := t.resolve(step.Target)
		if why != "" {
			return &InvalidError{App: t.App, Reasons: []string{why}}
		}
		x, y, ok := want.Center()
		if !ok {
			continue
		}

		role, title, owned, err := lookupAt(pid, x, y)
		if err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
		if !owned {
			return &InvalidError{App: t.App, Reasons: []string{fmt.Sprintf(
				"step %d: (%d,%d) is covered by another application, so clicking there would not reach %s",
				i+1, x, y, t.App)}}
		}
		if !sameControl(want, role, title) {
			return &InvalidError{App: t.App, Reasons: []string{fmt.Sprintf(
				"step %d: expected the %s %q at (%d,%d) and found a %s %q",
				i+1, want.Role, want.Title, x, y, role, title)}}
		}
	}
	return nil
}

func sameControl(want Element, role, title string) bool {
	if !strings.EqualFold(strings.TrimSpace(role), strings.TrimSpace(want.Role)) {
		return false
	}
	if strings.TrimSpace(want.Title) == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(want.Title))
}

func bringForward(ctx context.Context, r Runner, app string) error {
	name, err := literal(app)
	if err != nil {
		return err
	}

	out, err := r.Run(ctx, fmt.Sprintf(forwardScript, name))
	if err != nil {
		return classify(err.Error(), err)
	}

	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return fmt.Errorf("uiact: unreadable reply (output %q): %w", truncate(string(out), 200), err)
	}
	if !envelope.OK {
		return classify(envelope.Error, fmt.Errorf("uiact: %s", envelope.Error))
	}
	return nil
}

const forwardScript = `(function () {
  const se = Application('System Events');
  const sa = Application.currentApplication();
  sa.includeStandardAdditions = true;
  function reply(f) {
    try { return JSON.stringify({ ok: true, value: f() }); }
    catch (e) { return JSON.stringify({ ok: false, error: String((e && e.message) || e) }); }
  }
  return reply(function () {
    const proc = se.applicationProcesses.byName(%s);
    if (!proc.frontmost()) {
      proc.frontmost = true;
      sa.delay(0.4);
    }
    return 'front';
  });
})();
`
