package cdp

import (
	"context"
	"encoding/json"
	"fmt"
)

// Target is a browser target: a page, an iframe, a worker.
type Target struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Attached bool   `json:"attached"`
}

// Targets enumerates targets over the WebSocket.
//
// This is authoritative; the /json/list HTTP endpoint is not, having been
// observed empty on Electron apps whose browser endpoint worked.
func (c *Conn) Targets(ctx context.Context) ([]Target, error) {
	var res struct {
		TargetInfos []Target `json:"targetInfos"`
	}
	if err := c.Call(ctx, "", "Target.getTargets", nil, &res); err != nil {
		return nil, err
	}
	return res.TargetInfos, nil
}

// FirstPage returns the first target of type "page".
func (c *Conn) FirstPage(ctx context.Context) (Target, error) {
	targets, err := c.Targets(ctx)
	if err != nil {
		return Target{}, err
	}
	for _, t := range targets {
		if t.Type == "page" {
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("cdp: no page target among %d targets", len(targets))
}

// Attach opens a flattened session against a target and returns a Page.
func (c *Conn) Attach(ctx context.Context, targetID string) (*Page, error) {
	var res struct {
		SessionID string `json:"sessionId"`
	}
	params := map[string]any{"targetId": targetID, "flatten": true}
	if err := c.Call(ctx, "", "Target.attachToTarget", params, &res); err != nil {
		return nil, err
	}
	if res.SessionID == "" {
		return nil, fmt.Errorf("cdp: attach to %s returned no sessionId", targetID)
	}
	return &Page{conn: c, sessionID: res.SessionID}, nil
}

// Page drives one attached target.
type Page struct {
	conn      *Conn
	sessionID string
}

// SessionID is the CDP session this page speaks over.
func (p *Page) SessionID() string { return p.sessionID }

// Navigate loads url and waits for the page load event.
//
// Waiting for load is what makes navigation verifiable — a state change with
// an observable completion signal, rather than a fire-and-forget.
func (p *Page) Navigate(ctx context.Context, url string) error {
	if err := p.conn.Call(ctx, p.sessionID, "Page.enable", nil, nil); err != nil {
		return err
	}

	// Subscribe before navigating, or the load event can arrive first.
	events, unsub := p.conn.Subscribe()
	defer unsub()

	var res struct {
		FrameID   string `json:"frameId"`
		ErrorText string `json:"errorText"`
	}
	if err := p.conn.Call(ctx, p.sessionID, "Page.navigate", map[string]any{"url": url}, &res); err != nil {
		return err
	}
	if res.ErrorText != "" {
		return fmt.Errorf("cdp: navigate to %s failed: %s", url, res.ErrorText)
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cdp: navigate to %s: %w", url, ctx.Err())

		case <-p.conn.Done():
			return p.conn.closedErr()

		case m, ok := <-events:
			if !ok {
				return p.conn.closedErr()
			}
			if m.SessionID == p.sessionID && m.Method == "Page.loadEventFired" {
				return nil
			}
		}
	}
}

// OuterHTML returns the document's serialized markup.
func (p *Page) OuterHTML(ctx context.Context) (string, error) {
	return p.EvaluateString(ctx, "document.documentElement.outerHTML")
}

// URL returns the page's current location.
func (p *Page) URL(ctx context.Context) (string, error) {
	return p.EvaluateString(ctx, "document.location.href")
}

// EvaluateString runs a JavaScript expression and returns its string value.
func (p *Page) EvaluateString(ctx context.Context, expr string) (string, error) {
	var res struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}

	params := map[string]any{"expression": expr, "returnByValue": true}
	if err := p.conn.Call(ctx, p.sessionID, "Runtime.evaluate", params, &res); err != nil {
		return "", err
	}
	if res.ExceptionDetails != nil {
		return "", fmt.Errorf("cdp: evaluate threw: %s", res.ExceptionDetails.Text)
	}

	var s string
	if err := json.Unmarshal(res.Result.Value, &s); err != nil {
		return "", fmt.Errorf("cdp: evaluate returned %s, want string: %w", res.Result.Type, err)
	}
	return s, nil
}
