package webscreen

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/vedansh/vans/internal/names"
	"github.com/vedansh/vans/internal/uiact"
)

type Multi struct {
	Screens []*Screen
	Front   func(ctx context.Context) (string, error)

	mu   sync.Mutex
	last *Screen
}

func (m *Multi) named(app string) *Screen {
	if strings.TrimSpace(app) == "" {
		return nil
	}
	for _, s := range m.Screens {
		if names.Same(s.browser(), app) {
			return s
		}
	}
	return nil
}

func (m *Multi) Handles(app string) bool { return m.named(app) != nil }

func (m *Multi) pick(ctx context.Context, app string) *Screen {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s := m.named(app); s != nil {
		m.last = s
		return s
	}
	if m.Front != nil {
		if front, err := m.Front(ctx); err == nil {
			if s := m.named(front); s != nil {
				m.last = s
				return s
			}
		}
	}
	if m.last != nil {
		return m.last
	}
	if len(m.Screens) > 0 {
		return m.Screens[0]
	}
	return nil
}

var errNoBrowser = fmt.Errorf("webscreen: no browser is set up to be read")

func (m *Multi) Capture(ctx context.Context, app string) (uiact.Tree, error) {
	s := m.pick(ctx, app)
	if s == nil {
		return uiact.Tree{}, errNoBrowser
	}
	return s.Capture(ctx, app)
}

func (m *Multi) Act(ctx context.Context, p uiact.Program, t uiact.Tree) error {
	s := m.pick(ctx, "")
	if s == nil {
		return errNoBrowser
	}
	return s.Act(ctx, p, t)
}

func (m *Multi) Frontmost(ctx context.Context) (string, error) {
	s := m.pick(ctx, "")
	if s == nil {
		return "", errNoBrowser
	}
	return s.browser(), nil
}

func (m *Multi) BringToFront(ctx context.Context, app string) error {
	s := m.pick(ctx, app)
	if s == nil {
		return errNoBrowser
	}
	return s.BringToFront(ctx, app)
}

func (m *Multi) Type(ctx context.Context, text string) error {
	s := m.pick(ctx, "")
	if s == nil {
		return errNoBrowser
	}
	return s.Type(ctx, text)
}

func (m *Multi) ScrollIntoView(ctx context.Context, t uiact.Tree, sel uiact.Selector) error {
	s := m.pick(ctx, "")
	if s == nil {
		return errNoBrowser
	}
	return s.ScrollIntoView(ctx, t, sel)
}

func (m *Multi) Open(ctx context.Context, pageURL string) error {
	s := m.pick(ctx, "")
	if s == nil {
		return errNoBrowser
	}
	return s.Open(ctx, pageURL)
}

func (m *Multi) OpenIn(ctx context.Context, pageURL, profile string) error {
	s := m.pick(ctx, "")
	if s == nil {
		return errNoBrowser
	}
	return s.OpenIn(ctx, pageURL, profile)
}

func (m *Multi) Selection(ctx context.Context, app string) (string, bool) {
	s := m.pick(ctx, app)
	if s == nil {
		return "", false
	}
	return s.Selection(ctx, app)
}

func (m *Multi) ValueOf(ctx context.Context, t uiact.Tree, sel uiact.Selector) (string, bool) {
	s := m.pick(ctx, "")
	if s == nil {
		return "", false
	}
	return s.ValueOf(ctx, t, sel)
}

func (m *Multi) PutBack(ctx context.Context, app string, sel uiact.Selector, value string) error {
	s := m.pick(ctx, app)
	if s == nil {
		return errNoBrowser
	}
	return s.PutBack(ctx, app, sel, value)
}

func (m *Multi) ReadText(ctx context.Context, app string) (uiact.ScreenText, error) {
	s := m.pick(ctx, app)
	if s == nil {
		return uiact.ScreenText{}, errNoBrowser
	}
	return s.ReadText(ctx, app)
}
