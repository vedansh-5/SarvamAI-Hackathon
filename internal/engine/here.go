package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/store"
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
func (e *Engine) here(ctx context.Context, intent string) (*Plan, error) {
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
