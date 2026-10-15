package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/store"
)

type clicking struct {
	at  time.Time
	out []store.Click
}

func (c *clicking) do(app string, titles ...string) {
	for _, t := range titles {
		c.at = c.at.Add(3 * time.Second)
		c.out = append(c.out, store.Click{At: c.at, App: app, Role: "AXButton", Title: t})
	}
}

func (c *clicking) pause(d time.Duration) { c.at = c.at.Add(d) }

func TestARunDoneThreeTimesOnTwoOccasionsIsAHabit(t *testing.T) {
	c := &clicking{at: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)}
	c.do("Mail", "Archive", "Next", "Flag")
	c.do("Mail", "Reply")
	c.do("Mail", "Archive", "Next", "Flag")
	c.pause(time.Hour)
	c.do("Mail", "Archive", "Next", "Flag")
	c.do("Notes", "New Note")

	got := FindHabits(c.out)
	if len(got) != 1 {
		t.Fatalf("found %d habit(s): %+v, want the one Archive, Next, Flag", len(got), got)
	}
	if got[0].Name != "In Mail: Archive, Next, then Flag" {
		t.Errorf("named %q", got[0].Name)
	}
	if got[0].Times != 3 || got[0].Occasions != 2 {
		t.Errorf("times %d on %d occasion(s), want 3 on 2", got[0].Times, got[0].Occasions)
	}
}

func TestOneSittingIsNotAHabit(t *testing.T) {
	c := &clicking{at: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)}
	for i := 0; i < 5; i++ {
		c.do("Mail", "Archive", "Next", "Flag")
	}
	if got := FindHabits(c.out); len(got) != 0 {
		t.Errorf("found %+v in a single sitting, want nothing until it comes back another time", got)
	}
}

func TestTheSameButtonOverAndOverIsNotAHabit(t *testing.T) {
	c := &clicking{at: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)}
	for i := 0; i < 3; i++ {
		c.do("Calendar", "Next", "Next", "Next", "Next")
		c.pause(time.Hour)
	}
	if got := FindHabits(c.out); len(got) != 0 {
		t.Errorf("found %+v, want paging through to count for nothing", got)
	}
}

func TestTheLongestRunWinsOverThePartsInsideIt(t *testing.T) {
	c := &clicking{at: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)}
	for i := 0; i < 3; i++ {
		c.do("Finder", "File", "New Folder", "Share", "AirDrop")
		c.pause(time.Hour)
	}
	got := FindHabits(c.out)
	if len(got) != 1 || len(got[0].Steps) != 4 {
		t.Fatalf("found %+v, want only the four-click run, not its parts", got)
	}
}

func TestADoubleClickCountsOnce(t *testing.T) {
	c := &clicking{at: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)}
	for i := 0; i < 3; i++ {
		c.do("Mail", "Archive")
		c.out = append(c.out, store.Click{At: c.at.Add(200 * time.Millisecond), App: "Mail", Role: "AXButton", Title: "Archive"})
		c.do("Mail", "Next", "Flag")
		c.pause(time.Hour)
	}
	got := FindHabits(c.out)
	if len(got) != 1 || len(got[0].Steps) != 3 {
		t.Fatalf("found %+v, want Archive, Next, Flag with the double click as one", got)
	}
}

func TestAHabitAcrossAppsReadsAsSteps(t *testing.T) {
	steps := []store.Click{
		{App: "Mail", Role: "AXButton", Title: "Archive"},
		{App: "Notes", Role: "AXButton", Title: "New Note"},
		{App: "Notes", Role: "AXMenuItem", Title: "Paste"},
	}
	if got := habitName(steps); got != "In Mail: Archive, then in Notes: New Note, then Paste" {
		t.Errorf("named %q", got)
	}
	got := instructionFor(steps)
	if !strings.HasPrefix(got, "In Mail, click Archive, then in Notes, click New Note, then choose Paste") {
		t.Errorf("instruction %q", got)
	}
}

func TestAShortRunDoneFarMoreOftenStandsOnItsOwn(t *testing.T) {
	c := &clicking{at: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)}
	for i := 0; i < 3; i++ {
		c.do("Finder", "File", "New Folder", "Share", "AirDrop")
		c.pause(time.Hour)
	}
	for i := 0; i < 4; i++ {
		c.do("Finder", "File", "New Folder", "Share")
		c.pause(time.Hour)
	}
	got := FindHabits(c.out)
	if len(got) != 2 || len(got[1].Steps) != 3 || got[1].Times != 7 {
		t.Fatalf("found %+v, want the four-click run and, on its own, the three-click run done seven times", got)
	}
}
