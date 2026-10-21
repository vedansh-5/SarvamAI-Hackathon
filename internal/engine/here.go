package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/runner"
	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/uiact"
)

// FrontReader reads the page the user is looking at, rather than the one the
// runtime navigated. Implemented by the live browser adapter.
type FrontReader interface {
	Front(ctx context.Context) (url, html string, err error)
}

// here answers a request about the page already on screen.
//
// It navigates nowhere. The user supplied the page by having it open, so
// going anywhere would be answering a different question — and would lose the
// state that made the page worth asking about: a search they had refined, a
// list they had filtered, anything behind a login.
func (e *Engine) readWhatIsInFront(ctx context.Context, intent string, lookingAt *screen.Context) (*Plan, error) {
	if plan, ok := e.hereInFrontApp(ctx, intent, lookingAt); ok {
		return plan, nil
	}

	reader, ok := e.Browser.(FrontReader)
	if !ok {
		if lf, isLive := e.Browser.(*LiveFirst); isLive {
			reader, ok = lf.Live.(FrontReader)
		}
	}
	if !ok {
		return nil, fmt.Errorf("engine: this build cannot read the page in front of you")
	}

	if l, isLock := e.Browser.(sync.Locker); isLock {
		l.Lock()
		defer l.Unlock()
	}

	pageURL, src, err := reader.Front(ctx)
	if err != nil {
		return nil, fmt.Errorf("engine: read the page in front of you: %w", err)
	}
	src = e.googleFileIn(ctx, reader, pageURL, src)

	sig, err := extract.SignatureOf(src, extract.NewSigOptions())
	if err != nil {
		return nil, fmt.Errorf("engine: sign page: %w", err)
	}
	key, err := store.NewKey(pageURL, sig, intent)
	if err != nil {
		return nil, err
	}
	e.logf("reading %s, signature %s", pageURL, sig)

	return e.extractFrom(ctx, src, pageURL, sig, key, Task{
		URL:    pageURL,
		Intent: intent,
	})
}

var windowFields = []string{"kind", "label", "text"}

func (e *Engine) hereInFrontApp(ctx context.Context, intent string, lookingAt *screen.Context) (*Plan, bool) {
	if e.UIScreen == nil {
		return nil, false
	}

	app, err := e.appInQuestion(ctx, lookingAt)
	if err != nil || strings.TrimSpace(app) == "" {
		return nil, false
	}
	if isBrowser(app) {
		return nil, false
	}

	tree, err := e.UIScreen.Capture(ctx, app)
	if err != nil {
		e.logf("could not read %s's window: %v", app, err)
		return nil, false
	}

	rows := windowRows(tree)
	if len(rows) == 0 {
		return nil, false
	}

	title := strings.TrimSpace(tree.Window)
	if title == "" {
		title = app
	}
	e.logf("reading %s, window %q, %d readable controls", app, title, len(rows))

	return &Plan{
		Task:   Task{Intent: intent, Title: title},
		Fields: windowFields,
		Rows:   rows,
	}, true
}

func windowRows(t uiact.Tree) []runner.Row {
	var rows []runner.Row
	for _, el := range t.Elements {
		label := strings.TrimSpace(el.Title)
		text := strings.TrimSpace(el.Value)
		if label == "" && text == "" {
			continue
		}
		rows = append(rows, runner.Row{"kind": el.Role, "label": label, "text": text})
	}
	return rows
}
