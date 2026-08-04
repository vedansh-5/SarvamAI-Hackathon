package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// DefaultReadLimit is generous on purpose.
//
// coder/websocket defaults to 32 KiB, and a single Runtime.evaluate returning
// document.documentElement.outerHTML routinely exceeds that on real pages. Too
// small a limit fails the connection mid-read rather than returning a short
// result, which is a confusing way to discover the problem.
const DefaultReadLimit = 128 << 20 // 128 MiB

// wsTransport carries CDP frames over a WebSocket.
type wsTransport struct {
	c *websocket.Conn

	writeMu sync.Mutex // coder/websocket permits one writer at a time
}

func (w *wsTransport) Send(ctx context.Context, data []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.c.Write(ctx, websocket.MessageText, data)
}

func (w *wsTransport) Receive(ctx context.Context) ([]byte, error) {
	_, data, err := w.c.Read(ctx)
	return data, err
}

func (w *wsTransport) Close() error {
	return w.c.Close(websocket.StatusNormalClosure, "")
}

// Dial connects to a CDP WebSocket endpoint and starts the read loop.
func Dial(ctx context.Context, wsURL string) (*Conn, error) {
	ws, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp: dial %s: %w", wsURL, err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	ws.SetReadLimit(DefaultReadLimit)

	c := NewConn(&wsTransport{c: ws})
	c.Start(context.WithoutCancel(ctx))
	return c, nil
}

// BrowserWSURL asks the debug port for the browser-level WebSocket endpoint.
//
// The /json/version endpoint is used rather than /json/list because Electron
// apps have been observed serving an empty target list over HTTP while the
// browser endpoint works correctly. Enumerate targets over the WebSocket with
// Target.getTargets instead.
func BrowserWSURL(ctx context.Context, hostPort string) (string, error) {
	url := "http://" + hostPort + "/json/version"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("cdp: probe %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cdp: probe %s: status %d", url, resp.StatusCode)
	}

	var v struct {
		Browser              string `json:"Browser"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", fmt.Errorf("cdp: decode %s: %w", url, err)
	}
	if v.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("cdp: %s exposed no webSocketDebuggerUrl", url)
	}
	return v.WebSocketDebuggerURL, nil
}
