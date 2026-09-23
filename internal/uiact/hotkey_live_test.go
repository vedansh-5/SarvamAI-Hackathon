//go:build darwin && cgo && integration

package uiact

import (
	"os"
	"strings"
	"testing"
	"time"
)

// The line the shell prints from inside its shortcut handler.
const panelSawTheHotkey = "hotkey fired"

// TestPostedHotkeyReachesThePanelLive settles whether the keyboard can be driven
// from outside, by reading the panel's own log.
//
// Run with the app up and its output going to a file:
//
//	cd ui && npm run tauri dev > /tmp/vans-run.log 2>&1 &
//	VANS_PANEL_LOG=/tmp/vans-run.log go test -tags integration \
//	  -run TestPostedHotkeyReachesThePanelLive ./internal/uiact/
//
// The first attempt at this measurement asked the accessibility API whether VANS
// was publishing a window, concluded the keypress had not arrived, and was wrong
// in a way worth remembering: the panel is non-activating and belongs to an
// application that is deliberately never the active one, which is exactly the
// case macOS withholds from that API. The question was unanswerable, not the
// answer negative. The log is the only witness that is not subject to it.
func TestPostedHotkeyReachesThePanelLive(t *testing.T) {
	path := os.Getenv("VANS_PANEL_LOG")
	if path == "" {
		t.Skip("VANS_PANEL_LOG is not set; run the app with its output going to a file")
	}
	if !strings.Contains(read(t, path), "registered = true") {
		t.Skip("the app in that log never registered the hotkey; another application may own it")
	}

	before := strings.Count(read(t, path), panelSawTheHotkey)

	postKey(Keys["Space"], Modifiers["Option"])

	// Generous: the shortcut arrives on the main thread, behind whatever the
	// webview is doing. A second was enough every time it was measured.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if strings.Count(read(t, path), panelSawTheHotkey) > before {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the panel never logged %q after a posted Option-Space", panelSawTheHotkey)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
