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
	pid, err := pidFor(ctx, r, app)
	if err != nil {
		return Tree{}, err
	}
	return walkApp(pid, app)
}

func pidFor(ctx context.Context, r Runner, app string) (int, error) {
	name, err := literal(app)
	if err != nil {
		return 0, err
	}

	out, err := r.Run(ctx, fmt.Sprintf(pidScript, name))
	if err != nil {
		return 0, classify(err.Error(), err)
	}

	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Value int    `json:"value"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return 0, fmt.Errorf("uiact: unreadable reply (output %q): %w", truncate(string(out), 200), err)
	}
	if !envelope.OK {
		return 0, classify(envelope.Error, fmt.Errorf("uiact: %s", envelope.Error))
	}
	if envelope.Value <= 0 {
		return 0, fmt.Errorf("uiact: %s is not running", app)
	}
	return envelope.Value, nil
}

const pidScript = `(function () {
  const se = Application('System Events');
  function reply(f) {
    try { return JSON.stringify({ ok: true, value: f() }); }
    catch (e) { return JSON.stringify({ ok: false, error: String((e && e.message) || e) }); }
  }
  return reply(function () {
    return se.applicationProcesses.byName(%s).unixId();
  });
})();
`

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
    let front = [];
    try { front = se.applicationProcesses.whose({ frontmost: true })(); } catch (e) {}
    if (front.length > 0) return String(front[0].name());
    const procs = se.applicationProcesses();
    for (let i = 0; i < procs.length; i++) {
      try { if (procs[i].frontmost()) return String(procs[i].name()); } catch (e) {}
    }
    throw new Error('no application is in front');
  });
})();
`
