package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

func timedOffer(t *testing.T, trigger Trigger, action store.Click) (*Engine, *store.Store, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &Engine{Clicks: st}
	timed := Timed{Trigger: trigger, Action: action, Days: 3}
	timed.Key = timed.key()
	if err := e.keepTimed(context.Background(), timed); err != nil {
		t.Fatal(err)
	}
	return e, st, timed.Key
}

func TestAnAppHabitIsOfferedTheFirstTimeTheAppComesForwardToday(t *testing.T) {
	ctx := context.Background()
	e, st, _ := timedOffer(t, Trigger{Kind: AfterAnApp, App: "Mail"}, click(time.Time{}, "Clockify", "Start"))
	now := time.Now()

	_ = st.NoteClick(ctx, front(now, "Mail"))
	got, err := e.OffersFor(ctx, Cause{Kind: AppForward, App: "Mail"}, now)
	if err != nil || len(got) != 1 || got[0].Line != "Start in Clockify, like usual?" {
		t.Fatalf("offers = %+v, err = %v", got, err)
	}
	if got[0].Always || got[0].Countdown {
		t.Errorf("offered Always or a countdown before it was ever taken up: %+v", got[0])
	}

	_ = st.NoteClick(ctx, front(now.Add(time.Minute), "Mail"))
	if again, _ := e.OffersFor(ctx, Cause{Kind: AppForward, App: "Mail"}, now.Add(time.Minute)); len(again) != 0 {
		t.Errorf("offered again the same day: %+v", again)
	}
}

func TestNothingIsOfferedForWhatWasAlreadyDoneToday(t *testing.T) {
	ctx := context.Background()
	e, st, _ := timedOffer(t, Trigger{Kind: AfterAnApp, App: "Mail"}, click(time.Time{}, "Clockify", "Start"))
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	_ = st.NoteClick(ctx, click(now.Add(-time.Hour), "Clockify", "Start"))
	_ = st.NoteClick(ctx, front(now, "Mail"))
	if got, _ := e.OffersFor(ctx, Cause{Kind: AppForward, App: "Mail"}, now); len(got) != 0 {
		t.Errorf("offered %+v, though the timer was already started today", got)
	}
}

func TestTrustGrowsAndACancelTakesItBack(t *testing.T) {
	ctx := context.Background()
	e, st, key := timedOffer(t, Trigger{Kind: WhenOpened}, front(time.Time{}, "Spotify"))
	now := time.Now()

	for i := 0; i < yesesToTrust; i++ {
		got, err := e.AnswerOffer(ctx, key, OfferRun, now)
		if err != nil || got.Open != "Spotify" {
			t.Fatalf("run %d: %+v, %v; want Spotify opened", i+1, got, err)
		}
	}
	_ = st.SetSetting(ctx, greetedKey, dayOf(now))
	offers, _ := e.OffersFor(ctx, Cause{Kind: MacOpened}, now)
	if len(offers) != 1 || !offers[0].Always {
		t.Fatalf("offers = %+v, want Always offered after %d runs", offers, yesesToTrust)
	}

	if _, err := e.AnswerOffer(ctx, key, OfferAlways, now); err != nil {
		t.Fatal(err)
	}
	tomorrow := now.Add(24 * time.Hour)
	offers, _ = e.OffersFor(ctx, Cause{Kind: MacOpened}, tomorrow)
	if len(offers) != 1 || !offers[0].Countdown {
		t.Fatalf("offers = %+v, want the countdown once Always is on", offers)
	}

	if _, err := e.AnswerOffer(ctx, key, OfferCancel, tomorrow); err != nil {
		t.Fatal(err)
	}
	offers, _ = e.OffersFor(ctx, Cause{Kind: MacOpened}, tomorrow.Add(24*time.Hour))
	if len(offers) != 1 || offers[0].Countdown {
		t.Errorf("offers = %+v, want it asked again after Cancel, not counted down", offers)
	}
}

func TestThreeDismissalsInARowMakeItQuietForTheDay(t *testing.T) {
	ctx := context.Background()
	e, _, key := timedOffer(t, Trigger{Kind: AtATime, Minute: 9 * 60}, front(time.Time{}, "Spotify"))
	now := time.Date(2026, 10, 15, 9, 2, 0, 0, time.Local)

	for i := 0; i < quietAfter; i++ {
		if _, err := e.AnswerOffer(ctx, key, OfferIgnored, now); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := e.OffersFor(ctx, Cause{Kind: ByTheClock}, now); len(got) != 0 {
		t.Errorf("offered %+v on a day it was told three times to go away", got)
	}
	if got, _ := e.OffersFor(ctx, Cause{Kind: ByTheClock}, now.Add(24*time.Hour)); len(got) != 1 {
		t.Errorf("offered %+v the next day, want it back", got)
	}
}

func TestLaterBringsItBackAfterTheSnooze(t *testing.T) {
	ctx := context.Background()
	e, _, key := timedOffer(t, Trigger{Kind: AtATime, Minute: 9 * 60}, front(time.Time{}, "Spotify"))
	now := time.Date(2026, 10, 15, 9, 2, 0, 0, time.Local)

	if got, _ := e.OffersFor(ctx, Cause{Kind: ByTheClock}, now); len(got) != 1 {
		t.Fatalf("not offered at nine: %+v", got)
	}
	if _, err := e.AnswerOffer(ctx, key, OfferLater, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.OffersFor(ctx, Cause{Kind: ByTheClock}, now.Add(5*time.Minute)); len(got) != 0 {
		t.Errorf("came back after five minutes: %+v", got)
	}
	if got, _ := e.OffersFor(ctx, Cause{Kind: ByTheClock}, now.Add(snooze+time.Minute)); len(got) != 1 {
		t.Errorf("did not come back after the snooze: %+v", got)
	}
}

func TestGoodMorningListsWhatIsUsualOnceADay(t *testing.T) {
	ctx := context.Background()
	e, _, _ := timedOffer(t, Trigger{Kind: AtATime, Minute: 9*60 + 5}, front(time.Time{}, "Spotify"))
	now := time.Date(2026, 10, 15, 8, 0, 0, 0, time.Local)

	got, _ := e.OffersFor(ctx, Cause{Kind: MacOpened}, now)
	if len(got) != 1 || !got[0].Morning || len(got[0].Usual) != 1 || got[0].Usual[0] != "Open Spotify around 09:05" {
		t.Fatalf("offers = %+v, want a good morning naming Spotify around 09:05", got)
	}
	if again, _ := e.OffersFor(ctx, Cause{Kind: MacOpened}, now.Add(time.Hour)); len(again) != 0 {
		t.Errorf("said good morning twice: %+v", again)
	}
}

func TestSkippingThreeTimesStopsTheSuggestion(t *testing.T) {
	ctx := context.Background()
	e, st, key := timedOffer(t, Trigger{Kind: WhenOpened}, front(time.Time{}, "Spotify"))
	now := time.Now()
	for i := 0; i < skipsToStop; i++ {
		_ = st.SetSetting(ctx, quietKey, "")
		if _, err := e.AnswerOffer(ctx, key, OfferSkip, now); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.SetSetting(ctx, quietKey, "")
	_ = st.SetSetting(ctx, greetedKey, dayOf(now))
	if got, _ := e.OffersFor(ctx, Cause{Kind: MacOpened}, now.Add(48*time.Hour)); len(got) != 0 {
		t.Errorf("still offered after three skips: %+v", got)
	}
}

type learnedBox struct{ sent []synth.Learned }

func (b *learnedBox) SendLearned(_ context.Context, l synth.Learned) error {
	b.sent = append(b.sent, l)
	return nil
}

func TestWhatWasLearnedIsSentOnlyWhenItChanges(t *testing.T) {
	ctx := context.Background()
	e, _, key := timedOffer(t, Trigger{Kind: AfterAnApp, App: "Mail"}, click(time.Time{}, "Clockify", "Start"))
	box := &learnedBox{}
	e.Learner = box

	for i := 0; i < 2; i++ {
		if err := e.ShareWhatWasLearned(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(box.sent) != 1 || box.sent[0].Habits[0].What != "Start in Clockify" || box.sent[0].Habits[0].When != "after you open Mail" {
		t.Fatalf("sent %+v, want the one habit once", box.sent)
	}

	if _, err := e.AnswerOffer(ctx, key, OfferRun, time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = e.ShareWhatWasLearned(ctx)
	if len(box.sent) != 2 || box.sent[1].Habits[0].Yes != 1 {
		t.Errorf("sent %+v, want it sent again once it was taken up", box.sent)
	}
}

func TestBeforeYouFinishIsOfferedJustBeforeTheUsualTimeEvenIfDoneThisMorning(t *testing.T) {
	ctx := context.Background()
	e, st, _ := timedOffer(t, Trigger{Kind: BeforeDone, Minute: 18 * 60}, click(time.Time{}, "Clockify", "Stop"))
	morning := time.Date(2026, 10, 15, 11, 0, 0, 0, time.Local)
	_ = st.NoteClick(ctx, click(morning, "Clockify", "Stop"))
	_ = st.SetSetting(ctx, greetedKey, dayOf(morning))

	if got, _ := e.OffersFor(ctx, Cause{Kind: ByTheClock}, morning.Add(6*time.Hour+40*time.Minute)); len(got) != 0 {
		t.Errorf("offered at 17:40, too early: %+v", got)
	}
	got, _ := e.OffersFor(ctx, Cause{Kind: ByTheClock}, morning.Add(6*time.Hour+52*time.Minute))
	if len(got) != 1 || got[0].Line != "Before you finish, stop in Clockify, like usual?" {
		t.Fatalf("at 17:52 offered %+v", got)
	}
}

func TestBeforeYouFinishIsNotOfferedIfJustDone(t *testing.T) {
	ctx := context.Background()
	e, st, _ := timedOffer(t, Trigger{Kind: BeforeDone, Minute: 18 * 60}, click(time.Time{}, "Clockify", "Stop"))
	now := time.Date(2026, 10, 15, 17, 55, 0, 0, time.Local)
	_ = st.NoteClick(ctx, click(now.Add(-10*time.Minute), "Clockify", "Stop"))
	_ = st.SetSetting(ctx, greetedKey, dayOf(now))
	if got, _ := e.OffersFor(ctx, Cause{Kind: ByTheClock}, now); len(got) != 0 {
		t.Errorf("offered what was done ten minutes ago: %+v", got)
	}
}

func TestAfterABreakIsOfferedEachTimeYouComeBackUnlessItIsStillRunning(t *testing.T) {
	ctx := context.Background()
	e, st, _ := timedOffer(t, Trigger{Kind: AfterABreak}, click(time.Time{}, "Clock", "Start"))
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.Local)
	_ = st.SetSetting(ctx, greetedKey, dayOf(at))
	back := func(after time.Duration, kind string) []Offer {
		t.Helper()
		at = at.Add(after)
		_ = st.NoteClick(ctx, moment(at, kind))
		got, err := e.OffersFor(ctx, Cause{Kind: MacOpened}, at)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	press := func(title string) {
		at = at.Add(time.Minute)
		_ = st.NoteClick(ctx, click(at, "Clock", title))
	}

	_ = st.NoteClick(ctx, moment(at.Add(-time.Hour), "lock"))
	if got := back(0, "wake"); len(got) != 1 || got[0].Line != "Start in Clock, like usual?" {
		t.Fatalf("coming back after an hour offered %+v", got)
	}
	if got := back(2*time.Second, "unlock"); len(got) != 0 {
		t.Errorf("the unlock right after the wake offered it again: %+v", got)
	}

	press("Start")
	_ = st.NoteClick(ctx, moment(at.Add(time.Hour), "lock"))
	if got := back(2*time.Hour, "unlock"); len(got) != 0 {
		t.Errorf("offered while the stopwatch was still running: %+v", got)
	}

	press("Stop")
	_ = st.NoteClick(ctx, moment(at.Add(time.Minute), "lock"))
	if got := back(5*time.Minute, "unlock"); len(got) != 0 {
		t.Errorf("offered after a four-minute lock: %+v", got)
	}
	_ = st.NoteClick(ctx, moment(at.Add(time.Minute), "lock"))
	if got := back(30*time.Minute, "unlock"); len(got) != 1 {
		t.Errorf("coming back a second time today offered %+v", got)
	}
}
