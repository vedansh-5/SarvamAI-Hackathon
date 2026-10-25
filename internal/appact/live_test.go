package appact

import (
	"context"
	"os"
	"testing"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/capability"
)

// Run with VANS_LIVE=1. It writes to Notes and Reminders on this machine, and
// raises the automation prompt the first time.
func live(t *testing.T, name string) capability.Surface {
	t.Helper()
	if os.Getenv("VANS_LIVE") == "" {
		t.Skip("set VANS_LIVE=1 to drive real applications")
	}
	c := capability.New()
	app, err := c.Find(name)
	if err != nil {
		t.Skipf("%s is not installed", name)
	}
	d, err := c.Dictionary(app)
	if err != nil {
		t.Fatal(err)
	}
	return capability.Describe(app, d)
}

func TestLiveWriteToNotes(t *testing.T) {
	s := live(t, "Notes")

	p := Program{App: "Notes", Steps: []Step{{
		Command: "make",
		Make:    "note",
		With:    map[string]string{"body": "<div>Vspk live test</div>"},
		Bind:    "note",
	}}}

	t.Logf("preview: %s", Preview(p, s))

	got, err := Run(context.Background(), appleevents.Osascript{}, p, s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !got.Verified {
		t.Errorf("unverified receipt: %+v", got)
	}
	t.Logf("receipt: id=%s name=%q verified=%v", got.ID, got.Name, got.Verified)
}

func TestLiveReadFromReminders(t *testing.T) {
	s := live(t, "Reminders")

	p := Program{App: "Reminders", Steps: []Step{{
		Command: "get",
		Target:  &Selector{Class: "list", Index: 1},
		Bind:    "list",
	}}}

	if Writes(p, s) {
		t.Fatal("reading a list is reported as a write")
	}
	t.Logf("preview: %s", Preview(p, s))

	got, err := Run(context.Background(), appleevents.Osascript{}, p, s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Logf("receipt: id=%s name=%q value=%q verified=%v", got.ID, got.Name, got.Value, got.Verified)
}
