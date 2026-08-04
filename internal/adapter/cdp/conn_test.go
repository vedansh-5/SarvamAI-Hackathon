package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// fakeTransport is an in-memory CDP peer. It lets the protocol logic be tested
// with no WebSocket and no browser, which is most of what this package does.
type fakeTransport struct {
	sent     chan []byte
	incoming chan []byte
	closed   chan struct{}
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		sent:     make(chan []byte, 16),
		incoming: make(chan []byte, 16),
		closed:   make(chan struct{}),
	}
}

func (f *fakeTransport) Send(_ context.Context, data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	f.sent <- cp
	return nil
}

func (f *fakeTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.closed:
		return nil, errors.New("transport closed")
	case d := <-f.incoming:
		return d, nil
	}
}

func (f *fakeTransport) Close() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	return nil
}

// reply reads the next command and answers it with the given result JSON.
func (f *fakeTransport) reply(t *testing.T, resultJSON string) Message {
	t.Helper()
	select {
	case raw := <-f.sent:
		var req Message
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("bad command frame: %v", err)
		}
		f.incoming <- []byte(`{"id":` + itoa(req.ID) + `,"result":` + resultJSON + `}`)
		return req
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a command")
		return Message{}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func newTestConn(t *testing.T) (*Conn, *fakeTransport) {
	t.Helper()
	ft := newFakeTransport()
	c := NewConn(ft)
	c.Start(context.Background())
	t.Cleanup(func() { _ = c.Close() })
	return c, ft
}

func TestCallCorrelatesResponse(t *testing.T) {
	c, ft := newTestConn(t)

	type result struct {
		Value string `json:"value"`
	}
	var got result

	done := make(chan error, 1)
	go func() {
		done <- c.Call(context.Background(), "sess-1", "Some.method", map[string]any{"a": 1}, &got)
	}()

	req := ft.reply(t, `{"value":"ok"}`)

	if err := <-done; err != nil {
		t.Fatalf("Call: %v", err)
	}
	if req.Method != "Some.method" {
		t.Errorf("method = %q", req.Method)
	}
	if req.SessionID != "sess-1" {
		t.Errorf("sessionId = %q", req.SessionID)
	}
	if got.Value != "ok" {
		t.Errorf("result = %+v", got)
	}
}

func TestCallSurfacesRPCError(t *testing.T) {
	c, ft := newTestConn(t)

	done := make(chan error, 1)
	go func() {
		done <- c.Call(context.Background(), "", "Bad.method", nil, nil)
	}()

	raw := <-ft.sent
	var req Message
	_ = json.Unmarshal(raw, &req)
	ft.incoming <- []byte(`{"id":` + itoa(req.ID) + `,"error":{"code":-32000,"message":"nope"}}`)

	err := <-done
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("want *RPCError, got %v", err)
	}
	if rpcErr.Code != -32000 {
		t.Errorf("code = %d", rpcErr.Code)
	}
}

// Concurrent calls must not cross their responses — the id map is the only
// thing keeping them apart.
func TestConcurrentCallsDoNotCross(t *testing.T) {
	c, ft := newTestConn(t)

	const n = 8
	results := make([]string, n)
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		go func(i int) {
			var r struct {
				Value string `json:"value"`
			}
			err := c.Call(context.Background(), "", "M", map[string]any{"i": i}, &r)
			results[i] = r.Value
			errs <- err
		}(i)
	}

	// Answer each command with its own id echoed back as the value.
	for i := 0; i < n; i++ {
		raw := <-ft.sent
		var req Message
		_ = json.Unmarshal(raw, &req)
		ft.incoming <- []byte(`{"id":` + itoa(req.ID) + `,"result":{"value":"` + itoa(req.ID) + `"}}`)
	}

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("Call: %v", err)
		}
	}

	seen := map[string]bool{}
	for _, v := range results {
		if v == "" {
			t.Fatal("a call got no result")
		}
		if seen[v] {
			t.Fatalf("duplicate result %q — responses crossed", v)
		}
		seen[v] = true
	}
}

func TestSubscribeReceivesEvents(t *testing.T) {
	c, ft := newTestConn(t)

	events, unsub := c.Subscribe()
	defer unsub()

	ft.incoming <- []byte(`{"method":"Page.loadEventFired","sessionId":"s1"}`)

	select {
	case m := <-events:
		if m.Method != "Page.loadEventFired" || m.SessionID != "s1" {
			t.Errorf("got %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event delivered")
	}
}

// A dropped transport must fail in-flight calls rather than hang. This is the
// Electron self-update case: the app can revoke the debug port mid-task.
func TestTransportDropFailsPendingCalls(t *testing.T) {
	c, ft := newTestConn(t)

	done := make(chan error, 1)
	go func() {
		done <- c.Call(context.Background(), "", "Slow.method", nil, nil)
	}()

	<-ft.sent // command is in flight
	_ = ft.Close()

	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("want ErrClosed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending call hung after transport drop")
	}
}

func TestCallAfterCloseFails(t *testing.T) {
	c, _ := newTestConn(t)
	_ = c.Close()

	if err := c.Call(context.Background(), "", "M", nil, nil); !errors.Is(err, ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}

func TestCallRespectsContext(t *testing.T) {
	c, ft := newTestConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.Call(ctx, "", "Never.answered", nil, nil) }()

	<-ft.sent // sent, but never answered

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("want DeadlineExceeded, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Call ignored its context")
	}
}
