package server

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
)

func TestWhatWasLearnedIsKeptPerTesterAndNotCountedOrMailed(t *testing.T) {
	mail := &postbox{}
	srv, ledger := feedbackServer(t, 1, mail)
	remote := synth.NewRemote(srv.URL, riya)

	for _, what := range []string{"Start in Clockify", "Open Spotify"} {
		l := synth.Learned{Habits: []synth.LearnedHabit{{What: what, When: "after you open Mail", Days: 4, Yes: 3}}}
		if err := remote.SendLearned(context.Background(), l); err != nil {
			t.Fatalf("sending %q: %v", what, err)
		}
	}
	got := ledger.Learned("Riya")
	if !strings.Contains(got, "Open Spotify") || strings.Contains(got, "Clockify") {
		t.Errorf("kept %q, want only the latest list", got)
	}
	if len(mail.subjects) != 0 {
		t.Errorf("mailed %q; what was learned is not mailed", mail.subjects)
	}
	if err := remote.SendFeedback(context.Background(), synth.Feedback{Kind: synth.FeedbackNote, Text: "hi"}); err != nil {
		t.Errorf("a daily allowance of one was used up by sending what was learned: %v", err)
	}
}
