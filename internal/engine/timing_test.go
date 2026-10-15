package engine

import (
	"testing"
	"time"

	"github.com/vedansh/vans/internal/store"
)

func day(n, hour, minute int) time.Time {
	return time.Date(2026, 10, 12+n, hour, minute, 0, 0, time.Local)
}

func front(at time.Time, app string) store.Click {
	return store.Click{At: at, App: app, Role: store.RoleFront}
}

func moment(at time.Time, kind string) store.Click {
	return store.Click{At: at, App: store.MacApp, Role: store.RoleMoment, Title: kind}
}

func click(at time.Time, app, title string) store.Click {
	return store.Click{At: at, App: app, Role: "AXButton", Title: title}
}

func TestSomethingDoneAtTheSameTimeOnThreeWeekdaysIsTimed(t *testing.T) {
	var events []store.Click
	for i, m := range []int{5, 12, 0} {
		events = append(events, front(day(i, 9, m), "Spotify"))
	}
	got := FindTimed(events)
	if len(got) != 1 || got[0].Trigger.Kind != AtATime || !got[0].Trigger.Weekdays {
		t.Fatalf("found %+v, want Spotify around nine on weekdays", got)
	}
	if got[0].When() != "around 09:05 on weekdays" || got[0].Line() != "Open Spotify, like usual?" {
		t.Errorf("said %q / %q", got[0].When(), got[0].Line())
	}
}

func TestTimesTooFarApartAreNotTimed(t *testing.T) {
	events := []store.Click{front(day(0, 9, 0), "Spotify"), front(day(1, 13, 0), "Spotify"), front(day(2, 18, 0), "Spotify")}
	if got := FindTimed(events); len(got) != 0 {
		t.Errorf("found %+v in times hours apart", got)
	}
}

func TestSomethingDoneRightAfterOpeningTheMacIsTimed(t *testing.T) {
	var events []store.Click
	for i, h := range []int{7, 10, 13} {
		events = append(events, moment(day(i, h, 0), "unlock"), click(day(i, h, 3), "Calendar", "Today"))
	}
	got := FindTimed(events)
	if len(got) != 1 || got[0].Trigger.Kind != WhenOpened {
		t.Fatalf("found %+v, want Today in Calendar after the Mac is opened", got)
	}
	if got[0].When() != "after you open your Mac" || got[0].Instruction() != "In Calendar, click Today" {
		t.Errorf("said %q / %q", got[0].When(), got[0].Instruction())
	}
}

func TestSomethingDoneAfterAnAppComesForwardIsTimed(t *testing.T) {
	var events []store.Click
	for i, h := range []int{8, 11, 15} {
		events = append(events, front(day(i, h, 0), "Mail"), click(day(i, h, 2), "Clockify", "Start"))
	}
	events = append(events, front(day(3, 9, 0), "Mail"))
	got := FindTimed(events)
	if len(got) != 1 || got[0].Trigger.Kind != AfterAnApp || got[0].Trigger.App != "Mail" {
		t.Fatalf("found %+v, want Start in Clockify after Mail", got)
	}
	if got[0].Line() != "Start in Clockify, like usual?" {
		t.Errorf("line %q", got[0].Line())
	}
}

func TestAnAppThatIsOnlySometimesFollowedIsNotTimed(t *testing.T) {
	var events []store.Click
	for i := 0; i < 6; i++ {
		events = append(events, front(day(i, 8+i, 0), "Mail"))
		if i < 3 {
			events = append(events, click(day(i, 8+i, 2), "Clockify", "Start"))
		}
	}
	for _, got := range FindTimed(events) {
		if got.Trigger.Kind == AfterAnApp {
			t.Errorf("found %+v, though Start followed Mail on only half the days", got)
		}
	}
}
