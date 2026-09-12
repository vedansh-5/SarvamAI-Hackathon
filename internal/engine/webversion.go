package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/webact"
)

type OwnTabDriver interface {
	Navigate(ctx context.Context, pageURL string) error
	URL(ctx context.Context) (string, error)
	OuterHTML(ctx context.Context) (string, error)
	DoOnOwnTab(ctx context.Context, script string) error
}

var webVersions = map[string]string{
	"whatsapp":  "https://web.whatsapp.com",
	"notion":    "https://www.notion.so",
	"slack":     "https://app.slack.com/client",
	"telegram":  "https://web.telegram.org/k/",
	"discord":   "https://discord.com/app",
	"spotify":   "https://open.spotify.com",
	"gmail":     "https://mail.google.com/mail/u/0/",
	"figma":     "https://www.figma.com/files",
	"linear":    "https://linear.app",
	"trello":    "https://trello.com",
	"asana":     "https://app.asana.com",
	"messenger": "https://www.messenger.com",
}

func webVersionOf(app string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(app))
	key = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(key)
	if key == "" {
		return "", false
	}
	target, ok := webVersions[key]
	return target, ok
}

func (e *Engine) ownTabDriver() (OwnTabDriver, bool) {
	if d, ok := e.Browser.(OwnTabDriver); ok {
		return d, true
	}
	if lf, ok := e.Browser.(*LiveFirst); ok {
		if d, ok := lf.Live.(OwnTabDriver); ok {
			return d, true
		}
	}
	return nil, false
}

func (e *Engine) actOnWebVersion(ctx context.Context, r route.Route, shape, app string) (*Plan, int, error) {
	target, known := webVersionOf(app)
	if !known {
		return nil, 0, fmt.Errorf("engine: no web version known for %s", app)
	}

	driver, ok := e.ownTabDriver()
	if !ok || e.WebWriter == nil {
		return nil, 0, fmt.Errorf("engine: this build cannot drive a web application")
	}

	if l, isLock := e.Browser.(sync.Locker); isLock {
		l.Lock()
		defer l.Unlock()
	}

	if err := driver.Navigate(ctx, target); err != nil {
		return nil, 0, err
	}
	pageURL, err := driver.URL(ctx)
	if err != nil {
		return nil, 0, err
	}
	src, err := driver.OuterHTML(ctx)
	if err != nil {
		return nil, 0, err
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
				Why:         fmt.Sprintf("no way to do that in %s on the web survived being checked against it", app),
				Can:         e.capabilities(),
			}
		}
		return nil, calls, err
	}

	return &Plan{
		Task:           Task{Intent: r.Slot("action"), Title: app},
		PageURL:        pageURL,
		Destination:    hostOf(pageURL),
		SynthesisCalls: calls,
		Web: &WebPlan{
			URL:     pageURL,
			Preview: webact.Preview(filled),
			program: filled,
			script:  script,
			ownTab:  true,
		},
	}, calls, nil
}
