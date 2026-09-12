package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/webact"
	"golang.org/x/net/html"
)

// WebActionWriter writes a program against a page. Implemented by the
// synthesis backend.
type WebActionWriter interface {
	WriteWebAction(ctx context.Context, req synth.WebActRequest) (webact.Program, error)
}

// PageDriver runs a program in the tab the user is looking at.
type PageDriver interface {
	FrontReader
	Do(ctx context.Context, script string) error
}

// WebPlan is a prepared change inside a page.
type WebPlan struct {
	URL     string
	Preview string

	program webact.Program
	script  string
	ownTab  bool
}

// actOnPage prepares an action in the page the user has open.
//
// The second rung of the ladder for driving an application. The first is its
// scripting dictionary, which is faster and says plainly what changes state —
// but Telegram, Notion, Discord and Slack ship none, and every one of them
// has a web version already open in the browser VANS drives. Going through
// the page costs no new permission and no new dependency.
func (e *Engine) actOnPage(ctx context.Context, r route.Route, shape, wantApp string) (*Plan, int, error) {
	driver, ok := e.pageDriver()
	if !ok || e.WebWriter == nil {
		return nil, 0, fmt.Errorf("engine: this build cannot act in a page")
	}

	if l, isLock := e.Browser.(sync.Locker); isLock {
		l.Lock()
		defer l.Unlock()
	}

	pageURL, src, err := driver.Front(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("engine: read the page in front of you: %w", err)
	}

	if wantApp != "" && !pageBelongsTo(pageURL, wantApp) {
		return nil, 0, fmt.Errorf(
			"engine: the page in front is %s, not %s", hostOf(pageURL), wantApp)
	}

	doc, err := webact.Parse(src)
	if err != nil {
		return nil, 0, err
	}

	controls := webact.ControlsText(webact.Controls(doc), webact.MaxControlsBytes)

	filled, script, calls, err := e.writeWebProgram(ctx, r, shape, pageURL, controls, doc)
	if err != nil {
		if errors.Is(err, errNoWebProgram) {
			return nil, calls, &UnroutableError{
				Instruction: r.Slot("action"),
				Why:         "no way to do that on this page survived being checked against it",
				Can:         e.capabilities(),
			}
		}
		return nil, calls, err
	}

	return &Plan{
		Task:           Task{Intent: r.Slot("action"), Title: pageURL},
		PageURL:        pageURL,
		Destination:    hostOf(pageURL),
		SynthesisCalls: calls,
		Web: &WebPlan{
			URL:     pageURL,
			Preview: webact.Preview(filled),
			program: filled,
			script:  script,
		},
	}, calls, nil
}

// runWeb is the side effect. Reached only through Commit.
func (e *Engine) runWeb(ctx context.Context, plan *Plan) (Receipt, error) {
	driver, ok := e.pageDriver()
	if !ok {
		return Receipt{}, fmt.Errorf("engine: this build cannot act in a page")
	}

	if l, isLock := e.Browser.(sync.Locker); isLock {
		l.Lock()
		defer l.Unlock()
	}

	if plan.Web.ownTab {
		own, ok := e.ownTabDriver()
		if !ok {
			return Receipt{}, fmt.Errorf("engine: this build cannot act in its own tab")
		}
		if err := own.DoOnOwnTab(ctx, plan.Web.script); err != nil {
			return Receipt{}, err
		}
	} else if err := driver.Do(ctx, plan.Web.script); err != nil {
		return Receipt{}, err
	}

	e.logf("ran %d step(s) on %s", len(plan.Web.program.Steps), plan.Web.URL)

	// Nothing to read back. A page states no identity for what a click did,
	// so there is no object to ask about afterwards — unlike an application,
	// which hands back a specifier (D-004). Reported as unverified rather
	// than assumed, because the difference is real.
	return Receipt{Title: plan.Web.URL, Verified: false}, nil
}

func (e *Engine) pageDriver() (PageDriver, bool) {
	if d, ok := e.Browser.(PageDriver); ok {
		return d, true
	}
	if lf, ok := e.Browser.(*LiveFirst); ok {
		if d, ok := lf.Live.(PageDriver); ok {
			return d, true
		}
	}
	return nil, false
}

func hostOf(pageURL string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(pageURL, "https://"), "http://")
	if i := strings.IndexAny(trimmed, "/?#"); i > 0 {
		trimmed = trimmed[:i]
	}
	return strings.TrimPrefix(trimmed, "www.")
}

func pageBelongsTo(pageURL, app string) bool {
	host := strings.ToLower(hostOf(pageURL))
	host = strings.NewReplacer(".", "", "-", "", "_", "").Replace(host)

	name := strings.ToLower(strings.TrimSpace(app))
	name = strings.NewReplacer(" ", "", ".", "", "-", "", "_", "").Replace(name)
	if name == "" {
		return false
	}
	return strings.Contains(host, name)
}

var errNoWebProgram = errors.New("engine: no web program survived checking")

func (e *Engine) writeWebProgram(ctx context.Context, r route.Route, shape, pageURL, controls string, doc *html.Node) (webact.Program, string, int, error) {
	action := r.Template("action")
	if action == "" {
		action = r.Slot("action")
	}

	var failures []string
	budget := e.maxSynthesis()

	for attempt := 1; attempt <= budget; attempt++ {
		program, err := e.WebWriter.WriteWebAction(ctx, synth.WebActRequest{
			Action:       action,
			URL:          pageURL,
			Controls:     controls,
			Placeholders: route.PlaceholdersIn(shape),
			Failures:     failures,
		})
		if err != nil {
			return webact.Program{}, "", attempt, err
		}

		var rejected []string
		if err := webact.Validate(program, doc); err != nil {
			var invalid *webact.InvalidError
			if !errors.As(err, &invalid) {
				return webact.Program{}, "", attempt, err
			}
			rejected = invalid.Reasons
		}
		for _, baked := range webact.Baked(program, r.Literals) {
			rejected = append(rejected, fmt.Sprintf(
				"it types %q into the program instead of the placeholder standing for it; "+
					"this program is reused for every request of this shape", baked))
		}
		if len(rejected) > 0 {
			e.logf("page program rejected: %s", strings.Join(rejected, "; "))
			failures = rejected
			continue
		}

		filled := program.Fill(func(v string) string { return route.Fill(v, r.Literals) })
		script, err := webact.Render(filled, doc)
		if err != nil {
			return webact.Program{}, "", attempt, err
		}
		return filled, script, attempt, nil
	}

	return webact.Program{}, "", budget, errNoWebProgram
}
