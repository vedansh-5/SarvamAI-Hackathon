package webscreen

import (
	"context"
	"testing"
)

func twoBrowsers(front *string) (*Multi, *fakePage, *fakePage) {
	chrome := &fakePage{snap: flights()}
	other := flights()
	other.Host, other.Title = "apple.com", "Apple"
	safari := &fakePage{snap: other}
	m := &Multi{
		Screens: []*Screen{{Page: chrome, Browser: "Google Chrome"}, {Page: safari, Browser: "Safari"}},
		Front:   func(context.Context) (string, error) { return *front, nil },
	}
	return m, chrome, safari
}

func TestThePageIsReadFromTheBrowserInFront(t *testing.T) {
	quick(t)
	front := "Safari"
	m, _, _ := twoBrowsers(&front)
	ctx := context.Background()

	tree, err := m.Capture(ctx, "")
	if err != nil || tree.App != "apple.com" {
		t.Fatalf("read %q (%v), want Safari's page", tree.App, err)
	}
	if name, _ := m.Frontmost(ctx); name != "Safari" {
		t.Errorf("browser = %q", name)
	}

	front = "Google Chrome"
	if tree, _ := m.Capture(ctx, ""); tree.App != "example.com" {
		t.Errorf("read %q after Chrome came forward", tree.App)
	}
}

func TestABrowserNamedOutrightIsUsedWhateverIsInFront(t *testing.T) {
	quick(t)
	front := "Google Chrome"
	m, _, _ := twoBrowsers(&front)
	if tree, _ := m.Capture(context.Background(), "safari"); tree.App != "apple.com" {
		t.Errorf("read %q, want the browser that was asked for", tree.App)
	}
	if !m.Handles("Safari") || m.Handles("Firefox") {
		t.Error("says it can read a browser it has no way into, or cannot read one it has")
	}
}

func TestWithSomethingElseInFrontTheLastBrowserUsedIsKept(t *testing.T) {
	quick(t)
	front := "Safari"
	m, _, _ := twoBrowsers(&front)
	ctx := context.Background()
	_, _ = m.Capture(ctx, "")

	front = "vans"
	if tree, _ := m.Capture(ctx, ""); tree.App != "apple.com" {
		t.Errorf("read %q once the panel was in front; a run must stay in the browser it started in", tree.App)
	}
}
