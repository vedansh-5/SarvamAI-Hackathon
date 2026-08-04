//go:build integration

package appleevents

import (
	"context"
	"strings"
	"testing"
)

// Run with: go test -tags integration ./internal/adapter/appleevents/
//
// This creates a real note in Notes.app and triggers the macOS Automation
// permission prompt the first time. Excluded from the default suite so
// `go test ./...` stays hermetic.
func TestCreateVerifiedLive(t *testing.T) {
	n := Notes{Runner: Osascript{}}

	got, err := n.CreateVerified(context.Background(), "<div><h1>VANS test</h1><div>hello</div></div>")
	if err != nil {
		if IsPermissionDenied(err) {
			t.Fatal("Automation permission denied — grant it in System Settings > Privacy & Security > Automation")
		}
		t.Fatalf("CreateVerified: %v", err)
	}

	if got.ID == "" {
		t.Error("no id returned")
	}
	if !strings.Contains(got.Plaintext, "hello") {
		t.Errorf("read-back missing body content: %+v", got)
	}
	t.Logf("created note %s (%q) — delete it manually", got.ID, got.Name)
}
