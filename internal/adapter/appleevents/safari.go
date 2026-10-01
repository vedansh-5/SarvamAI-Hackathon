package appleevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Safari struct {
	Runner Runner
}

var ErrSafariScriptingOff = errors.New("appleevents: Safari is not allowing JavaScript from Apple Events")

var ErrSafariNotRunning = errors.New("appleevents: Safari is not running")

const SafariScriptingFix = "In Safari: Settings \u203A Advanced \u203A Show features for web developers, then Develop \u203A Allow JavaScript from Apple Events."

const safariPreamble = `function safari() {
  const app = Application('Safari');
  if (!app.running()) throw new Error('Safari is not running');
  if (app.windows.length === 0) throw new Error('Safari has no window open');
  return app;
}
function reply(f) {
  try { return JSON.stringify({ ok: true, value: f() }); }
  catch (e) { return JSON.stringify({ ok: false, error: String(e && e.message || e) }); }
}
`

const stillLoading = "__vans_loading__"

func (s *Safari) run(ctx context.Context, script string, out any) error {
	raw, err := s.Runner.Run(ctx, safariPreamble+script)
	if err != nil {
		if strings.Contains(err.Error(), "Allow JavaScript from Apple Events") {
			return ErrSafariScriptingOff
		}
		return err
	}
	var envelope struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("appleevents: unreadable reply from Safari: %w (output %q)", err, raw)
	}
	if !envelope.OK {
		switch {
		case strings.Contains(envelope.Error, "Allow JavaScript from Apple Events"):
			return ErrSafariScriptingOff
		case strings.Contains(envelope.Error, "not running"):
			return ErrSafariNotRunning
		}
		return fmt.Errorf("appleevents: Safari: %s", envelope.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Value, out)
}

func (s *Safari) eval(ctx context.Context, expression string) (string, error) {
	source, err := jsLiteral(expression)
	if err != nil {
		return "", err
	}
	var raw string
	err = s.run(ctx, fmt.Sprintf(`reply(function () {
  const app = safari();
  return String(app.doJavaScript(%s, { in: app.windows[0].currentTab }));
});`, source), &raw)
	return raw, err
}

func (s *Safari) AskFront(ctx context.Context, script string, out any) error {
	raw, err := s.eval(ctx, script)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("appleevents: the page in front cannot be read or driven (it answered %q)", truncate(raw, 80))
	}
	return nil
}

func (s *Safari) ReadFront(ctx context.Context, script string, out any) (bool, error) {
	raw, err := s.eval(ctx, "document.readyState === 'loading' ? '"+stillLoading+"' : ("+script+")")
	if err != nil {
		return false, err
	}
	if raw == stillLoading {
		return false, nil
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return false, fmt.Errorf("appleevents: the page in front cannot be read or driven (it answered %q)", truncate(raw, 80))
	}
	return true, nil
}

func (s *Safari) OpenInFront(ctx context.Context, pageURL string) error {
	target, err := jsLiteral(normaliseURL(pageURL))
	if err != nil {
		return err
	}
	return s.run(ctx, fmt.Sprintf(`reply(function () {
  const app = safari();
  const win = app.windows[0];
  const tab = app.Tab({ url: %s });
  win.tabs.push(tab);
  win.currentTab = tab;
  return true;
});`, target), nil)
}
