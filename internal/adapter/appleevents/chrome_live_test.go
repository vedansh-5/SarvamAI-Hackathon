package appleevents

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// Drives the browser actually open on this machine. VANS_LIVE=1 to run.
func TestLiveChrome(t *testing.T) {
	if os.Getenv("VANS_LIVE") == "" {
		t.Skip("set VANS_LIVE=1 to drive the running browser")
	}
	c := &Chrome{Runner: Osascript{}}
	ctx := context.Background()

	url, err := c.URL(ctx)
	if err != nil {
		if errors.Is(err, ErrNotRunning) {
			t.Skip("Chrome is not open")
		}
		t.Fatalf("URL: %v", err)
	}
	t.Logf("active tab: %s", url)

	html, err := c.OuterHTML(ctx)
	if err != nil {
		if errors.Is(err, ErrScriptingOff) {
			t.Skipf("scripting is off. %s", Fix(err))
		}
		t.Fatalf("OuterHTML: %v", err)
	}
	if !strings.Contains(strings.ToLower(html), "<html") {
		t.Errorf("that does not look like a document: %.120s", html)
	}
	t.Logf("read %d bytes of rendered DOM", len(html))
}
