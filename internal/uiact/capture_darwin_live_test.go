//go:build integration && darwin && cgo

package uiact

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/adapter/appleevents"
)

func pidOf(t *testing.T, app string) int {
	t.Helper()
	out, err := exec.Command("pgrep", "-x", app).Output()
	if err != nil {
		t.Skipf("%s is not running", app)
	}
	first := strings.Fields(strings.TrimSpace(string(out)))
	if len(first) == 0 {
		t.Skipf("%s is not running", app)
	}
	pid, err := strconv.Atoi(first[0])
	if err != nil {
		t.Skipf("unreadable pid for %s: %v", app, err)
	}
	return pid
}

func TestLiveWalkReadsANativeWindow(t *testing.T) {
	app := "Notes"
	pid := pidOf(t, app)

	start := time.Now()
	tree, err := walkApp(pid, app)
	took := time.Since(start)
	if err != nil {
		if Fix(err) != "" {
			t.Fatalf("%v — %s", err, Fix(err))
		}
		t.Fatalf("walkApp: %v", err)
	}

	if len(tree.Elements) == 0 {
		t.Fatalf("%s: walked a window with nothing in it", app)
	}
	t.Logf("%s — window %q, %d controls in %s", tree.App, tree.Window, len(tree.Elements), took.Round(time.Millisecond))

	withBox, typeable, titled := 0, 0, 0
	for _, el := range tree.Elements {
		if _, _, ok := el.Center(); ok {
			withBox++
		}
		if el.Typeable() {
			typeable++
		}
		if el.Title != "" {
			titled++
		}
	}
	t.Logf("clickable coordinates: %d/%d, typeable: %d, titled: %d",
		withBox, len(tree.Elements), typeable, titled)

	if withBox == 0 {
		t.Error("no control reported coordinates, so nothing could be clicked")
	}

	shown := 0
	for _, el := range tree.Elements {
		if el.Title == "" || shown >= 8 {
			continue
		}
		x, y, _ := el.Center()
		t.Logf("  %s %q @(%d,%d)", el.Role, el.Title, x, y)
		shown++
	}
}

func TestLiveCaptureThroughTheSystemScreen(t *testing.T) {
	pidOf(t, "Notes")

	screen := System{Runner: appleevents.Osascript{}}
	start := time.Now()
	tree, err := screen.Capture(context.Background(), "Notes")
	took := time.Since(start)
	if err != nil {
		if Fix(err) != "" {
			t.Fatalf("%v — %s", err, Fix(err))
		}
		t.Fatalf("Capture: %v", err)
	}

	if len(tree.Elements) == 0 {
		t.Fatal("the production path captured a window with nothing in it")
	}
	t.Logf("through System.Capture: %d controls in %s (pid lookup included)",
		len(tree.Elements), took.Round(time.Millisecond))

	if _, why := tree.TypingTarget(); why != "" {
		t.Logf("no typing target: %s", why)
	}
}
