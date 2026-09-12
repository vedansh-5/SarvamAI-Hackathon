//go:build integration

package uiact

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/adapter/appleevents"
)

func liveRunner() appleevents.Osascript {
	return appleevents.Osascript{Timeout: 120 * time.Second}
}

func liveApp(t *testing.T) (string, context.Context) {
	t.Helper()
	if os.Getenv("VANS_LIVE") == "" {
		t.Skip("set VANS_LIVE=1 to drive a real window")
	}
	app := os.Getenv("VANS_LIVE_APP")
	if app == "" {
		t.Skip("set VANS_LIVE_APP to the application to read, e.g. VANS_LIVE_APP=Notes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	return app, ctx
}

func TestLiveCaptureReadsAWindow(t *testing.T) {
	app, ctx := liveApp(t)

	tree, err := Capture(ctx, liveRunner(), app)
	if err != nil {
		if Fix(err) != "" {
			t.Fatalf("%v — %s", err, Fix(err))
		}
		t.Fatalf("Capture: %v", err)
	}

	if len(tree.Elements) == 0 {
		t.Fatalf("%s: captured a window with no controls in it", app)
	}
	t.Logf("%s — window %q, %d controls", tree.App, tree.Window, len(tree.Elements))

	typeables, pressables := 0, 0
	for _, el := range tree.Elements {
		if el.Typeable() {
			typeables++
		}
		if el.Pressable() && strings.EqualFold(el.Role, "AXButton") {
			pressables++
		}
	}
	t.Logf("controls text can be typed into: %d, buttons: %d", typeables, pressables)

	for _, el := range tree.Elements {
		if el.Typeable() {
			t.Logf("  typeable: %s %q value=%q", el.Role, el.Title, el.Value)
		}
	}

	shown := 0
	for _, el := range tree.Elements {
		if el.Title == "" || shown >= 12 {
			continue
		}
		t.Logf("  %s %q", el.Role, el.Title)
		shown++
	}
}

// Opt-in twice over: VANS_LIVE_TYPE names the control to type into, so this
// never puts text into something that happened to be on screen.
func TestLiveTypesIntoAWindow(t *testing.T) {
	app, ctx := liveApp(t)

	title := os.Getenv("VANS_LIVE_TYPE")
	if title == "" {
		t.Skip("set VANS_LIVE_TYPE to the title of a text control to type into")
	}

	tree, err := Capture(ctx, liveRunner(), app)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	var target Element
	for _, el := range tree.Elements {
		if el.Typeable() && strings.EqualFold(strings.TrimSpace(el.Title), title) {
			target = el
			break
		}
	}
	if target.Role == "" {
		t.Fatalf("%s has no control that can hold text named %q", app, title)
	}

	const typed = "vans live check"
	p := Program{App: tree.App, Steps: []Step{{
		Do:       "type",
		Target:   Selector{Role: target.Role, Title: target.Title},
		Text:     typed,
		Describe: "Type into " + target.Title,
	}}}

	if err := Validate(p, tree); err != nil {
		t.Fatalf("the program was refused before running: %v", err)
	}
	t.Logf("preview: %s", Preview(p))

	got, err := Run(ctx, liveRunner(), p, tree)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !got.Verified {
		t.Errorf("the text was typed but reading it back did not confirm it: %+v", got)
	}
	t.Logf("typed into %s %q, verified=%v", target.Role, target.Title, got.Verified)
}
