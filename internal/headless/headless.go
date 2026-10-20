package headless

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/adapter/cdp"
)

const chromeBinary = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

type Chrome struct {
	Ctx  context.Context
	tb   testing.TB
	conn *cdp.Conn
	page *cdp.Page
}

func Open(tb testing.TB) *Chrome {
	tb.Helper()
	bin := os.Getenv("VANS_CHROME")
	if bin == "" {
		bin = chromeBinary
	}
	if _, err := os.Stat(bin); err != nil {
		tb.Skipf("no Chrome at %s; set VANS_CHROME to one", bin)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	tb.Cleanup(cancel)

	profile := tb.TempDir()
	chrome := exec.Command(bin, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile,
		"--no-first-run", "--no-default-browser-check", "--window-size=1200,900", "about:blank")
	if err := chrome.Start(); err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = chrome.Process.Kill(); _ = chrome.Wait() })

	var port string
	for deadline := time.Now().Add(20 * time.Second); port == "" && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if b, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			port = strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
		}
	}
	if port == "" {
		tb.Fatal("headless Chrome never said which port it listens on")
	}
	ws, err := cdp.BrowserWSURL(ctx, "127.0.0.1:"+port)
	if err != nil {
		tb.Fatal(err)
	}
	conn, err := cdp.Dial(ctx, ws)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = conn.Close() })
	target, err := conn.FirstPage(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	page, err := conn.Attach(ctx, target.TargetID)
	if err != nil {
		tb.Fatal(err)
	}
	return &Chrome{Ctx: ctx, tb: tb, conn: conn, page: page}
}

func (c *Chrome) AskFront(ctx context.Context, script string, out any) error {
	raw, err := c.page.EvaluateString(ctx, script)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}

func (c *Chrome) ReadFront(ctx context.Context, script string, out any) (bool, error) {
	state, err := c.page.EvaluateString(ctx, "document.readyState")
	if err != nil {
		return false, err
	}
	if state != "complete" {
		return false, nil
	}
	return true, c.AskFront(ctx, script, out)
}

func (c *Chrome) OpenInFront(ctx context.Context, pageURL string) error {
	return c.page.Navigate(ctx, pageURL)
}

func (c *Chrome) Eval(script string) string {
	c.tb.Helper()
	out, err := c.page.EvaluateString(c.Ctx, script)
	if err != nil {
		c.tb.Fatalf("%s: %v", script, err)
	}
	return out
}

func (c *Chrome) mouse(kind string, x, y int) {
	var at []int
	if err := json.Unmarshal([]byte(c.Eval("JSON.stringify([screenX, screenY + Math.max(0, outerHeight - innerHeight)])")), &at); err != nil {
		c.tb.Fatal(err)
	}
	params := map[string]any{"type": kind, "x": x - at[0], "y": y - at[1], "button": "left", "clickCount": 1}
	if kind == "mouseMoved" {
		params["button"] = "none"
	}
	if err := c.conn.Call(c.Ctx, c.page.SessionID(), "Input.dispatchMouseEvent", params, nil); err != nil {
		c.tb.Fatal(err)
	}
}

func (c *Chrome) Click(x, y int) {
	c.mouse("mouseMoved", x, y)
	c.mouse("mousePressed", x, y)
	c.mouse("mouseReleased", x, y)
}

func (c *Chrome) Move(x, y int) { c.mouse("mouseMoved", x, y) }

func (c *Chrome) TypeText(text string) {
	if err := c.conn.Call(c.Ctx, c.page.SessionID(), "Input.insertText", map[string]any{"text": text}, nil); err != nil {
		c.tb.Fatal(err)
	}
}

func (c *Chrome) Drag(x1, y1, x2, y2 int) {
	c.mouse("mouseMoved", x1, y1)
	c.mouse("mousePressed", x1, y1)
	for i := 1; i <= 5; i++ {
		c.mouse("mouseMoved", x1+(x2-x1)*i/5, y1+(y2-y1)*i/5)
	}
	c.mouse("mouseReleased", x2, y2)
}

func Serve(tb testing.TB, dir string) (own, elsewhere string) {
	tb.Helper()
	files := http.FileServer(http.Dir(dir))
	other := httptest.NewServer(files)
	tb.Cleanup(other.Close)
	elsewhere = strings.Replace(other.URL, "127.0.0.1", "localhost", 1)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".html") {
			b, err := os.ReadFile(filepath.Join(dir, filepath.Clean(r.URL.Path)))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(strings.ReplaceAll(string(b), "OTHER", elsewhere)))
			return
		}
		files.ServeHTTP(w, r)
	}))
	tb.Cleanup(site.Close)
	return site.URL, elsewhere
}
