// Package cdp speaks the Chrome DevTools Protocol.
//
// It targets Chrome and every Electron application, which is why this is a
// Chromium adapter rather than a browser adapter — one implementation reaches
// Chrome, Notion, VS Code, Discord, and the rest.
//
// Connections are treated as revocable. An Electron app can drop the debug
// port at any moment through its own update cycle, so callers must be prepared
// to reconnect rather than assume a session they opened stays open.
package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// Transport carries raw CDP frames. Abstracted so the protocol logic can be
// tested without a WebSocket or a browser.
type Transport interface {
	Send(ctx context.Context, data []byte) error
	Receive(ctx context.Context) ([]byte, error)
	Close() error
}

// Message is a CDP frame: a command, a response, or an event.
type Message struct {
	ID        int             `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *RPCError       `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// RPCError is an error returned by the browser.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e.Data != "" {
		return fmt.Sprintf("cdp: %s (%d): %s", e.Message, e.Code, e.Data)
	}
	return fmt.Sprintf("cdp: %s (%d)", e.Message, e.Code)
}

// ErrClosed is returned once the connection has shut down.
var ErrClosed = errors.New("cdp: connection closed")

// Conn multiplexes commands and events over one Transport.
type Conn struct {
	t Transport

	mu      sync.Mutex
	nextID  int
	pending map[int]chan *Message
	subs    map[int]chan *Message
	nextSub int
	closed  bool
	cause   error

	done chan struct{}
}

// NewConn wraps a Transport. Call Start before issuing commands.
func NewConn(t Transport) *Conn {
	return &Conn{
		t:       t,
		pending: make(map[int]chan *Message),
		subs:    make(map[int]chan *Message),
		done:    make(chan struct{}),
	}
}

// Start launches the read loop. It returns immediately; the loop runs until
// the transport fails or Close is called.
func (c *Conn) Start(ctx context.Context) {
	go c.readLoop(ctx)
}

func (c *Conn) readLoop(ctx context.Context) {
	for {
		data, err := c.t.Receive(ctx)
		if err != nil {
			c.shutdown(err)
			return
		}

		var m Message
		if err := json.Unmarshal(data, &m); err != nil {
			continue // a frame we cannot parse is not a reason to tear down
		}

		c.dispatch(&m)
	}
}

func (c *Conn) dispatch(m *Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if m.ID != 0 {
		if ch, ok := c.pending[m.ID]; ok {
			delete(c.pending, m.ID)
			ch <- m
			close(ch)
		}
		return
	}

	// Events go to every subscriber. Non-blocking: a slow subscriber drops
	// frames rather than stalling the read loop for everyone.
	for _, ch := range c.subs {
		select {
		case ch <- m:
		default:
		}
	}
}

func (c *Conn) shutdown(cause error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.cause = cause

	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
	for id, ch := range c.subs {
		delete(c.subs, id)
		close(ch)
	}
	c.mu.Unlock()

	close(c.done)
	_ = c.t.Close()
}

// Call issues a command and decodes its result into out, which may be nil.
// sessionID may be empty to address the browser itself.
func (c *Conn) Call(ctx context.Context, sessionID, method string, params, out any) error {
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("cdp: encode params for %s: %w", method, err)
		}
		rawParams = b
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return c.closedErr()
	}
	c.nextID++
	id := c.nextID
	ch := make(chan *Message, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req := Message{ID: id, Method: method, Params: rawParams, SessionID: sessionID}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("cdp: encode %s: %w", method, err)
	}

	if err := c.t.Send(ctx, data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("cdp: send %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()

	case m, ok := <-ch:
		if !ok || m == nil {
			return c.closedErr()
		}
		if m.Error != nil {
			return m.Error
		}
		if out == nil || len(m.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(m.Result, out); err != nil {
			return fmt.Errorf("cdp: decode %s result: %w", method, err)
		}
		return nil
	}
}

// Subscribe returns a channel of events and a function to stop listening.
func (c *Conn) Subscribe() (<-chan *Message, func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan *Message, 64)
	if c.closed {
		close(ch)
		return ch, func() {}
	}

	c.nextSub++
	id := c.nextSub
	c.subs[id] = ch

	return ch, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if ch, ok := c.subs[id]; ok {
			delete(c.subs, id)
			close(ch)
		}
	}
}

// Close shuts the connection down.
func (c *Conn) Close() error {
	c.shutdown(nil)
	return nil
}

// Done is closed when the connection ends, for whatever reason.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err reports why the connection ended, or nil if it is still up or was
// closed deliberately.
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cause
}

func (c *Conn) closedErr() error {
	if c.cause != nil {
		return fmt.Errorf("%w: %v", ErrClosed, c.cause)
	}
	return ErrClosed
}
