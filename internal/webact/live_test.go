//go:build integration

package webact

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/adapter/appleevents"
)

func liveChrome(t *testing.T) (*appleevents.Chrome, context.Context) {
	t.Helper()
	if os.Getenv("VANS_LIVE") == "" {
		t.Skip("set VANS_LIVE=1 to read a real page")
	}

	c := &appleevents.Chrome{Runner: appleevents.Osascript{}, Timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	return c, ctx
}

func TestLiveControlsFromARealPage(t *testing.T) {
	c, ctx := liveChrome(t)

	if err := c.Navigate(ctx, "https://duckduckgo.com"); err != nil {
		if fix := appleevents.Fix(err); fix != "" {
			t.Skipf("%v — %s", err, fix)
		}
		t.Skipf("no browser to read: %v", err)
	}

	src, err := c.OuterHTML(ctx)
	if err != nil {
		t.Fatalf("read the page: %v", err)
	}
	t.Logf("page is %d bytes of HTML", len(src))

	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	found := Controls(doc)
	listing := ControlsText(found, MaxControlsBytes)
	t.Logf("distilled to %d controls, %d bytes (%.1f%% of the page)",
		len(found), len(listing), 100*float64(len(listing))/float64(len(src)))

	if len(found) == 0 {
		t.Fatal("a real page yielded no controls at all")
	}

	for _, c := range found {
		if !matchesOne(doc, c.Selector) {
			t.Errorf("selector %q does not address exactly one control", c.Selector)
		}
	}

	shown := 0
	for _, c := range found {
		if shown >= 8 {
			break
		}
		t.Logf("  %s (%s) %s", c.Selector, c.Kind, c.Label)
		shown++
	}

	if strings.Contains(listing, "<div") || strings.Contains(listing, "<script") {
		t.Error("raw markup reached the listing")
	}
}
