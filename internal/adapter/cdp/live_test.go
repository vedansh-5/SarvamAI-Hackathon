//go:build integration

package cdp

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Run with:
//
//	/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
//	  --remote-debugging-port=9222 --user-data-dir=/tmp/vans-chrome &
//	go test -tags integration ./internal/adapter/cdp/
//
// A separate --user-data-dir is used so this never touches your real profile.
func TestNavigateAndReadDOMLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wsURL, err := BrowserWSURL(ctx, "127.0.0.1:9222")
	if err != nil {
		t.Skipf("no Chrome on 127.0.0.1:9222: %v", err)
	}

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	target, err := conn.FirstPage(ctx)
	if err != nil {
		t.Fatalf("FirstPage: %v", err)
	}

	page, err := conn.Attach(ctx, target.TargetID)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	if err := page.Navigate(ctx, "https://example.com/"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	got, err := page.URL(ctx)
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	if !strings.Contains(got, "example.com") {
		t.Errorf("landed on %q", got)
	}

	html, err := page.OuterHTML(ctx)
	if err != nil {
		t.Fatalf("OuterHTML: %v", err)
	}
	if !strings.Contains(html, "<h1>Example Domain</h1>") {
		t.Errorf("DOM missing expected content (%d bytes)", len(html))
	}
	t.Logf("read %d bytes of DOM", len(html))
}
