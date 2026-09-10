package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/adapter/cdp"
)

// NotesSink writes documents into the macOS Notes application.
type NotesSink struct{ Notes appleevents.Notes }

// Write creates a note and reads it back.
//
// The read-back is what makes this a verified state change rather than an
// assumed one (D-013). If Notes reports a permission denial the error is
// returned as-is and deliberately not marked transient: a TCC prompt the user
// has not answered is not something retrying will fix, and retrying it forever
// is exactly how a runtime becomes a thing people force-quit.
func (s NotesSink) Destination() Destination {
	return Destination{
		Name:         "notes",
		Describe:     "Write it to a new note in Notes.",
		TouchesWorld: true,
	}
}

func (s NotesSink) Write(ctx context.Context, doc Document) (Receipt, error) {
	note, err := s.Notes.CreateVerified(ctx, doc.HTML())
	if err != nil {
		if appleevents.IsPermissionDenied(err) {
			return Receipt{}, fmt.Errorf("notes: permission denied — grant automation access in System Settings › Privacy & Security › Automation: %w", err)
		}
		return Receipt{}, err
	}
	return Receipt{ID: note.ID, Title: note.Name, Verified: true}, nil
}

// ChromiumBrowser drives an already-running Chromium target over CDP.
//
// It does not launch anything. Chrome must already be running with a debugging
// port open, and Connect says so plainly when it is not, because the fix is a
// command the user runs — not something the runtime should do to their machine
// on its own initiative.
type ChromiumBrowser struct {
	// HostPort is where the debugging endpoint is listening, e.g.
	// "127.0.0.1:9222".
	HostPort string

	mu   sync.Mutex
	conn *cdp.Conn
	page *cdp.Page
}

// NewChromiumBrowser returns a browser bound to a debugging endpoint. Nothing
// is dialled until the first operation.
func NewChromiumBrowser(hostPort string) *ChromiumBrowser {
	return &ChromiumBrowser{HostPort: hostPort}
}

// Close releases the connection.
func (b *ChromiumBrowser) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.drop()
}

func (b *ChromiumBrowser) drop() error {
	b.page = nil
	if b.conn == nil {
		return nil
	}
	conn := b.conn
	b.conn = nil
	return conn.Close()
}

// attach returns a live page, dialling or re-dialling as needed.
func (b *ChromiumBrowser) attach(ctx context.Context) (*cdp.Page, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn != nil && b.page != nil {
		select {
		case <-b.conn.Done():
			// The browser went away — a self-update and relaunch does this,
			// which was observed in practice (D-011). Fall through and redial.
			_ = b.drop()
		default:
			return b.page, nil
		}
	}

	wsURL, err := cdp.BrowserWSURL(ctx, b.HostPort)
	if err != nil {
		return nil, &TransientError{Err: fmt.Errorf(
			"no Chromium debugging endpoint at %s. Start Chrome yourself with --remote-debugging-port=%s; VANS does not launch applications: %w",
			b.HostPort, portOf(b.HostPort), err)}
	}

	conn, err := cdp.Dial(ctx, wsURL)
	if err != nil {
		return nil, &TransientError{Err: fmt.Errorf("cdp: dial %s: %w", wsURL, err)}
	}

	target, err := conn.FirstPage(ctx)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("cdp: no page target at %s: %w", b.HostPort, err)
	}

	page, err := conn.Attach(ctx, target.TargetID)
	if err != nil {
		conn.Close()
		return nil, &TransientError{Err: fmt.Errorf("cdp: attach: %w", err)}
	}

	b.conn, b.page = conn, page
	return page, nil
}

// do runs op against a live page, converting a lost connection into a
// transient error the engine's rung 0 knows how to retry.
func (b *ChromiumBrowser) do(ctx context.Context, op func(*cdp.Page) error) error {
	page, err := b.attach(ctx)
	if err != nil {
		return err
	}

	if err := op(page); err != nil {
		if errors.Is(err, cdp.ErrClosed) {
			b.mu.Lock()
			_ = b.drop()
			b.mu.Unlock()
			return &TransientError{Err: err}
		}
		return err
	}
	return nil
}

// Navigate loads pageURL and waits for the load event.
func (b *ChromiumBrowser) Navigate(ctx context.Context, pageURL string) error {
	return b.do(ctx, func(p *cdp.Page) error { return p.Navigate(ctx, pageURL) })
}

// URL reports where the page actually ended up.
func (b *ChromiumBrowser) URL(ctx context.Context) (string, error) {
	var out string
	err := b.do(ctx, func(p *cdp.Page) error {
		v, err := p.URL(ctx)
		out = v
		return err
	})
	return out, err
}

// OuterHTML captures the document.
func (b *ChromiumBrowser) OuterHTML(ctx context.Context) (string, error) {
	var out string
	err := b.do(ctx, func(p *cdp.Page) error {
		v, err := p.OuterHTML(ctx)
		out = v
		return err
	})
	return out, err
}

func portOf(hostPort string) string {
	for i := len(hostPort) - 1; i >= 0; i-- {
		if hostPort[i] == ':' {
			return hostPort[i+1:]
		}
	}
	return "9222"
}
