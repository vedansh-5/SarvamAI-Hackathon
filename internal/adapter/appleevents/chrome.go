package appleevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Chrome drives the browser the user is already running.
//
// The debugging port cannot reach it. Chrome 136 and later refuse
// --remote-debugging-port on the default profile — a deliberate change, since
// an open debugging port on a signed-in browser is every cookie in it — and
// passing the flag to a Chrome that is already running does nothing at all,
// because the second process forwards to the first and exits. Between them
// that leaves only one way to get CDP: a second Chrome on a throwaway profile,
// signed in to nothing.
//
// Apple Events go to the browser that is actually open. Nothing to launch,
// nothing to sign in to, and no port left listening afterwards.
type Chrome struct {
	Runner Runner

	// Timeout bounds a page load. Navigation is a poll, because Chrome's
	// dictionary offers no way to wait for one.
	Timeout time.Duration

	// nav serializes navigation and reading. Two requests sharing one browser
	// would otherwise read each other's pages (D-030).
	nav sync.Mutex

	// tab is the tab VANS opened for itself, by Chrome's id for it. Reused
	// across tasks so the browser does not fill up with them, and reopened if
	// the user closes it.
	tab string

	// Remember persists that id between runs. Without it every restart of the
	// engine opens another tab and abandons the last one, which is how a
	// browser ends up with nine of them.
	Remember TabMemory
}

// TabMemory keeps the tab id somewhere that outlives the process.
type TabMemory interface {
	Setting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// TabKey is where the tab id is kept.
const TabKey = "chrome.tab"

func (c *Chrome) recall(ctx context.Context) {
	if c.tab != "" || c.Remember == nil {
		return
	}
	if id, err := c.Remember.Setting(ctx, TabKey); err == nil {
		c.tab = id
	}
}

func (c *Chrome) remember(ctx context.Context) {
	if c.Remember == nil {
		return
	}
	_ = c.Remember.SetSetting(ctx, TabKey, c.tab)
}

func (c *Chrome) Lock()   { c.nav.Lock() }
func (c *Chrome) Unlock() { c.nav.Unlock() }

// ErrScriptingOff is Chrome refusing to run JavaScript sent by Apple Events.
//
// Off by default, and the one thing the user has to turn on. Its own error
// because the fix is a menu item and nothing else in the runtime can do it.
var ErrScriptingOff = errors.New("appleevents: Chrome is not allowing JavaScript from Apple Events")

// ErrNotRunning is Chrome not being open. VANS does not open it (D-032).
var ErrNotRunning = errors.New("appleevents: Chrome is not running")

const scriptingOffFix = "In Chrome: View › Developer › Allow JavaScript from Apple Events."

func (c *Chrome) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 20 * time.Second
}

// run executes a script and turns Chrome's refusals into typed errors.
func (c *Chrome) run(ctx context.Context, script string, out any) error {
	raw, err := c.Runner.Run(ctx, preamble+script)
	if err != nil {
		if strings.Contains(err.Error(), "Allow JavaScript from Apple Events") {
			return ErrScriptingOff
		}
		return err
	}

	var envelope struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("appleevents: unreadable reply from Chrome: %w (output %q)", err, raw)
	}
	if !envelope.OK {
		switch {
		case strings.Contains(envelope.Error, "Allow JavaScript from Apple Events"):
			return ErrScriptingOff
		case strings.Contains(envelope.Error, "not running"):
			return ErrNotRunning
		}
		return fmt.Errorf("appleevents: Chrome: %s", envelope.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Value, out)
}

// preamble is shared by every script: it finds the frontmost window and
// reports failures as data rather than as an osascript error, so the reason
// survives into Go instead of being flattened into an exit status.
const preamble = `function chrome() {
  const app = Application('Google Chrome');
  if (!app.running()) throw new Error('Chrome is not running');
  if (app.windows.length === 0) throw new Error('Chrome has no window open');
  return app;
}
function tabById(app, id) {
  for (let w = 0; w < app.windows.length; w++) {
    const tabs = app.windows[w].tabs;
    for (let i = 0; i < tabs.length; i++) {
      if (String(tabs[i].id()) === id) return tabs[i];
    }
  }
  throw new Error('the VANS tab is gone');
}
function reply(f) {
  try { return JSON.stringify({ ok: true, value: f() }); }
  catch (e) { return JSON.stringify({ ok: false, error: String(e && e.message || e) }); }
}
`

// Navigate opens VANS's own tab and sends it somewhere.
//
// Its own, never the one in front. The tab the user is reading is theirs, and
// taking it over to go and look something up is the rudest thing a background
// agent can do to a browser. The tab is opened behind whatever they are
// looking at, reused for every later request, and left addressable by id so a
// task always reads the page it asked for.
func (c *Chrome) Navigate(ctx context.Context, pageURL string) error {
	target, err := jsLiteral(normaliseURL(pageURL))
	if err != nil {
		return err
	}
	c.recall(ctx)

	if c.tab != "" {
		id, err := jsLiteral(c.tab)
		if err != nil {
			return err
		}
		err = c.run(ctx, fmt.Sprintf(`reply(function () {
  const app = chrome();
  const tab = tabById(app, %s);
  tab.url = %s;
  return String(tab.id());
});`, id, target), &c.tab)
		if err == nil {
			return c.settle(ctx)
		}
		if errors.Is(err, ErrScriptingOff) || errors.Is(err, ErrNotRunning) {
			return err
		}
		// Gone, or refused. Open a fresh one rather than failing the task.
		c.tab = ""
	}

	// Opened in the background: the active tab index is put back afterwards,
	// so the window the user is looking at does not change under them.
	err = c.run(ctx, fmt.Sprintf(`reply(function () {
  const app = chrome();
  const win = app.windows[0];
  const was = win.activeTabIndex();
  const tab = app.Tab({ url: %s });
  win.tabs.push(tab);
  win.activeTabIndex = was;
  return String(tab.id());
});`, target), &c.tab)
	if err != nil {
		return err
	}
	c.remember(ctx)

	return c.settle(ctx)
}

// normaliseURL supplies the scheme a person leaves out.
//
// The router hands over what the instruction said, and instructions say
// "news.ycombinator.com". CDP guesses; Chrome's dictionary refuses with
// "Invalid URL entered", which is true and unhelpful.
func normaliseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.Contains(trimmed, "://") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "about:") || strings.HasPrefix(trimmed, "chrome:") {
		return trimmed
	}
	return "https://" + trimmed
}

// settle waits for the tab to finish loading.
//
// Chrome's dictionary has a `loading` property on a tab, and it is the only
// signal available: there is no command that blocks until a page is ready.
// Polling it is what a person does when they watch the spinner.
func (c *Chrome) settle(ctx context.Context) error {
	deadline := time.Now().Add(c.timeout())

	for {
		var loading bool
		err := c.run(ctx, c.onTab("return tab.loading();"), &loading)

		// Older Chrome builds do not expose loading. A missing signal is not a
		// failed navigation, so fall back to a fixed pause rather than
		// refusing to work at all.
		if err != nil {
			if errors.Is(err, ErrScriptingOff) || errors.Is(err, ErrNotRunning) {
				return err
			}
			select {
			case <-time.After(1500 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if !loading {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("appleevents: the page was still loading after %s", c.timeout())
		}

		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// URL is where the frontmost tab actually is, which is not always where it was
// sent — redirects and consent walls both move it.
func (c *Chrome) URL(ctx context.Context) (string, error) {
	var url string
	err := c.run(ctx, c.onTab("return String(tab.url());"), &url)
	return url, err
}

// OuterHTML is the rendered DOM, not the source Chrome was served.
//
// Everything downstream signs and extracts against what is on screen, so a
// page that builds itself with script has to be read after it has done so.
func (c *Chrome) OuterHTML(ctx context.Context) (string, error) {
	var html string
	err := c.run(ctx, c.onTab(
		`return String(app.execute(tab, { javascript: 'document.documentElement.outerHTML' }));`),
		&html)
	return html, err
}

// onTab wraps a body so it runs against VANS's own tab, falling back to the
// frontmost one when there is not one yet — which is what Reachable asks
// before any task has opened one.
func (c *Chrome) onTab(body string) string {
	lookup := "const tab = app.windows[0].activeTab;"
	if c.tab != "" {
		id, err := jsLiteral(c.tab)
		if err == nil {
			lookup = "const tab = tabById(app, " + id + ");"
		}
	}
	return "reply(function () {\n  const app = chrome();\n  " + lookup + "\n  " + body + "\n});"
}

// Front is the page the user is looking at: the active tab of the frontmost
// window, never the tab VANS opened for itself.
//
// Deliberately separate from URL and OuterHTML, which address VANS's own tab.
// A request about "this page" means theirs, and reading the wrong one would
// answer confidently about a page they cannot see.
func (c *Chrome) Front(ctx context.Context) (url, html string, err error) {
	var front struct {
		URL   string `json:"url"`
		HTML  string `json:"html"`
		Title string `json:"title"`
	}

	err = c.run(ctx, `reply(function () {
  const app = chrome();
  const tab = app.windows[0].activeTab;
  return {
    url: String(tab.url()),
    title: String(tab.title()),
    html: String(app.execute(tab, { javascript: 'document.documentElement.outerHTML' })),
  };
});`, &front)
	if err != nil {
		return "", "", err
	}
	return front.URL, front.HTML, nil
}

// FrontTitle is what the frontmost tab calls itself. Read without executing
// anything, so it works before the scripting gate is opened.
func (c *Chrome) FrontTitle(ctx context.Context) (string, error) {
	var title string
	err := c.run(ctx, `reply(function () {
  return String(chrome().windows[0].activeTab.title());
});`, &title)
	return title, err
}

// Reachable reports whether this adapter can drive Chrome right now, and says
// what to do if it cannot.
func (c *Chrome) Reachable(ctx context.Context) error {
	_, err := c.OuterHTML(ctx)
	return err
}

// Fix is the instruction for an error from this adapter, or empty when there
// is nothing the user can do about it.
func Fix(err error) string {
	switch {
	case errors.Is(err, ErrScriptingOff):
		return scriptingOffFix
	case errors.Is(err, ErrNotRunning):
		return "Open Chrome. VANS does not launch applications."
	default:
		return ""
	}
}
