package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/vedansh/vans/internal/adapter/appleevents"
)

// LiveFirst prefers the browser the user is already using.
//
// The debugging port cannot reach that browser. Chrome 136 and later refuse
// --remote-debugging-port on the default profile, and passing the flag to a
// Chrome that is already running does nothing, because the second process
// hands off to the first and exits. So CDP means a second Chrome on a
// throwaway profile — signed in to nothing, which is the wrong browser for
// almost every question worth asking.
//
// Apple Events reach the real one. They need a menu item turned on, which is
// a thing the user does once, so the debugged browser stays as the fallback
// rather than being removed.
type LiveFirst struct {
	Live     Browser
	Debugged Browser

	mu     sync.Mutex
	chosen Browser
}

// Lock holds the underlying browser for the whole of one navigate-and-read,
// whichever one gets chosen. Both of them drive a single shared tab.
func (b *LiveFirst) Lock() {
	b.mu.Lock()
	if l, ok := b.Debugged.(sync.Locker); ok {
		l.Lock()
	}
	if l, ok := b.Live.(sync.Locker); ok {
		l.Lock()
	}
}

func (b *LiveFirst) Unlock() {
	if l, ok := b.Live.(sync.Locker); ok {
		l.Unlock()
	}
	if l, ok := b.Debugged.(sync.Locker); ok {
		l.Unlock()
	}
	b.mu.Unlock()
}

// Navigate chooses a browser and sends it somewhere. The choice is made here
// rather than lazily, because it is the first thing a task does and the rest
// of the task must read the same browser it navigated.
func (b *LiveFirst) Navigate(ctx context.Context, pageURL string) error {
	live, liveErr := b.tryLive(ctx, pageURL)
	if liveErr == nil {
		b.chosen = live
		return nil
	}

	if b.Debugged == nil {
		return liveErr
	}

	debuggedErr := b.Debugged.Navigate(ctx, pageURL)
	if debuggedErr == nil {
		b.chosen = b.Debugged
		return nil
	}

	// Actionable beats generic. "Turn on this menu item" is a fix; "no page
	// target among 0 targets" describes a second browser the user never
	// wanted to run.
	if actionable(liveErr) {
		return liveErr
	}

	// Neither worked and neither has an obvious fix. Both reasons go back:
	// the live browser is the one that is meant to work, and discarding its
	// error left only the fallback's, which says nothing about why the real
	// browser could not be driven.
	return fmt.Errorf("%w (the debugging port did not answer either: %v)", liveErr, debuggedErr)
}

func (b *LiveFirst) tryLive(ctx context.Context, pageURL string) (Browser, error) {
	if b.Live == nil {
		return nil, errors.New("engine: no live browser configured")
	}
	if err := b.Live.Navigate(ctx, pageURL); err != nil {
		return nil, err
	}
	return b.Live, nil
}

func (b *LiveFirst) URL(ctx context.Context) (string, error) {
	return b.current().URL(ctx)
}

func (b *LiveFirst) OuterHTML(ctx context.Context) (string, error) {
	return b.current().OuterHTML(ctx)
}

func (b *LiveFirst) current() Browser {
	if b.chosen != nil {
		return b.chosen
	}
	if b.Live != nil {
		return b.Live
	}
	return b.Debugged
}

// actionable reports whether an error names something the user can fix. Those
// are worth surfacing ahead of a generic failure from the fallback.
func actionable(err error) bool {
	return appleevents.Fix(err) != ""
}
