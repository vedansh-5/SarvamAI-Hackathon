package engine

import (
	"context"
	"time"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/uiact"
)

type ClickLog interface {
	Watching(ctx context.Context) (bool, error)
	SetWatching(ctx context.Context, on bool) error
	NoteClick(ctx context.Context, c store.Click) error
	Clicks(ctx context.Context, since time.Time) ([]store.Click, error)
	ForgetClicksBefore(ctx context.Context, before time.Time) error
	ClearClicks(ctx context.Context) error
}

const keepClicksFor = 7 * 24 * time.Hour

func (e *Engine) NoteClick(ctx context.Context, x, y int) (bool, error) {
	if e.Clicks == nil {
		return false, nil
	}
	on, err := e.Clicks.Watching(ctx)
	if err != nil || !on {
		return false, err
	}
	at := e.ClickAt
	if at == nil {
		at = uiact.NoticeAt
	}
	seen, ok := at(x, y)
	if !ok {
		return false, nil
	}
	now := time.Now()
	if err := e.Clicks.NoteClick(ctx, store.Click{At: now, App: seen.App, Role: seen.Role, Title: seen.Title}); err != nil {
		return false, err
	}
	if err := e.Clicks.ForgetClicksBefore(ctx, now.Add(-keepClicksFor)); err != nil {
		e.logf("could not drop clicks older than a week: %v", err)
	}
	return true, nil
}

func (e *Engine) NotedClicks(ctx context.Context) ([]store.Click, error) {
	if e.Clicks == nil {
		return nil, nil
	}
	return e.Clicks.Clicks(ctx, time.Now().Add(-keepClicksFor))
}
