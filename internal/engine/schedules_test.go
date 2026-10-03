package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/store"
)

func scheduling(t *testing.T, now time.Time) *Engine {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	was := clock
	clock = func() time.Time { return now }
	t.Cleanup(func() { clock = was })
	return &Engine{Schedules: st, Log: func(string, ...any) {}}
}

var fridayAfternoon = time.Date(2026, 10, 2, 16, 30, 0, 0, time.Local)

func scheduled(t *testing.T, e *Engine, instruction string) *Plan {
	t.Helper()
	plan, handled, err := e.aboutSchedules(context.Background(), instruction)
	if err != nil {
		t.Fatalf("%q: %v", instruction, err)
	}
	if !handled {
		t.Fatalf("%q was not taken as being about a schedule", instruction)
	}
	return plan
}

func TestATaskIsScheduledListedAndCancelled(t *testing.T) {
	e := scheduling(t, fridayAfternoon)

	plan := scheduled(t, e, `every weekday at 9am run "morning report"`)
	for _, want := range []string{"morning report", "every weekday at 9 am", "Mon 5 Oct", "does not start anything on its own"} {
		if !strings.Contains(plan.Say, want) {
			t.Errorf("want %q in what was said:\n%s", want, plan.Say)
		}
	}
	if plan.NeedsCommit() {
		t.Error("writing down a schedule was held for confirmation; nothing has been done yet")
	}

	if listed := scheduled(t, e, "schedules"); !strings.Contains(listed.Say, "morning report") {
		t.Errorf("the list does not show it:\n%s", listed.Say)
	}
	if gone := scheduled(t, e, `cancel schedule "morning report"`); !strings.Contains(gone.Say, "1 removed") {
		t.Errorf("say = %q, want one removed", gone.Say)
	}
	if listed := scheduled(t, e, "schedules"); !strings.Contains(listed.Say, "Nothing is scheduled") {
		t.Errorf("the list still shows something:\n%s", listed.Say)
	}
}

func TestOrdinaryRequestsAreNotTakenForSchedules(t *testing.T) {
	e := scheduling(t, fridayAfternoon)
	for _, in := range []string{
		"send hi to Joe on whatsapp",
		"every tab should be closed",
		"remind me how to open settings",
		"book a flight for tomorrow at 9am",
		"search for flights at 5pm",
	} {
		if _, handled, _ := e.aboutSchedules(context.Background(), in); handled {
			t.Errorf("%q was taken as a schedule", in)
		}
	}
	if all, _ := e.Schedules.Schedules(context.Background()); len(all) != 0 {
		t.Errorf("%d schedule(s) were made from requests that asked for none", len(all))
	}
}

func TestATimeWithNothingToDoAsksWhat(t *testing.T) {
	e := scheduling(t, fridayAfternoon)
	if plan := scheduled(t, e, "every day at 9am"); !strings.Contains(plan.Say, "what to do") {
		t.Errorf("say = %q, want to be asked what to do", plan.Say)
	}
}

func TestWhatIsDueIsBroughtUpOnceAndMovedOn(t *testing.T) {
	e := scheduling(t, fridayAfternoon)
	scheduled(t, e, `every day at 5pm tidy the desktop`)
	scheduled(t, e, `in 10 minutes remind me to stretch`)
	ctx := context.Background()

	if due := e.DueNow(ctx, fridayAfternoon.Add(5*time.Minute)); len(due) != 0 {
		t.Fatalf("brought up %+v before its time", due)
	}
	due := e.DueNow(ctx, fridayAfternoon.Add(45*time.Minute))
	if len(due) != 2 {
		t.Fatalf("brought up %+v, want both", due)
	}
	if again := e.DueNow(ctx, fridayAfternoon.Add(46*time.Minute)); len(again) != 0 {
		t.Errorf("brought up %+v a second time", again)
	}

	left, _ := e.Schedules.Schedules(ctx)
	if len(left) != 1 || left[0].What != "tidy the desktop" {
		t.Fatalf("left = %+v, want only the daily one, moved to tomorrow", left)
	}
	if want := time.Date(2026, 10, 3, 17, 0, 0, 0, time.Local); !left[0].Next.Equal(want) {
		t.Errorf("next = %v, want %v", left[0].Next, want)
	}
}

func TestSeveralMissedTimesAreBroughtUpAsOne(t *testing.T) {
	e := scheduling(t, fridayAfternoon)
	scheduled(t, e, `every day at 5pm tidy the desktop`)

	aWeekLater := fridayAfternoon.AddDate(0, 0, 7)
	if due := e.DueNow(context.Background(), aWeekLater); len(due) != 1 {
		t.Errorf("brought up %d time(s) for a week that was missed, want once", len(due))
	}
}
