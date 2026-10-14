package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
)

type postbox struct {
	subjects []string
	bodies   []string
	broken   error
}

func (p *postbox) Mail(_ context.Context, subject, body string) error {
	if p.broken != nil {
		return p.broken
	}
	p.subjects = append(p.subjects, subject)
	p.bodies = append(p.bodies, body)
	return nil
}

func feedbackServer(t *testing.T, daily int, mail *postbox) (*httptest.Server, *Memory) {
	t.Helper()
	ledger := NewMemory([]Tester{{Name: "Riya", Code: riya, Daily: daily}})
	srv := httptest.NewServer(&Server{
		Model:  func(*synth.Usage) synth.Model { return &standIn{} },
		Ledger: ledger,
		Mail:   mail,
		Now:    func() time.Time { return time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC) },
	})
	t.Cleanup(srv.Close)
	return srv, ledger
}

func TestFeedbackIsKeptAndMailed(t *testing.T) {
	mail := &postbox{}
	srv, ledger := feedbackServer(t, 10, mail)

	err := synth.NewRemote(srv.URL, riya).SendFeedback(context.Background(), synth.Feedback{Kind: synth.FeedbackFailedGuide, Text: "  The second step pointed at the wrong button.  "})
	if err != nil {
		t.Fatal(err)
	}
	kept := ledger.Kept()
	if len(kept) != 1 || kept[0].From != "Riya" || kept[0].Text != "The second step pointed at the wrong button." {
		t.Errorf("kept = %+v", kept)
	}
	if len(mail.subjects) != 1 || mail.subjects[0] != "VANS: failed guide from Riya" || !strings.Contains(mail.bodies[0], "wrong button") {
		t.Errorf("mailed %q / %q", mail.subjects, mail.bodies)
	}
}

func TestFeedbackIsKeptWhenTheMailFails(t *testing.T) {
	srv, ledger := feedbackServer(t, 10, &postbox{broken: errors.New("not verified")})

	if err := synth.NewRemote(srv.URL, riya).SendFeedback(context.Background(), synth.Feedback{Kind: synth.FeedbackNote, Text: "hello"}); err != nil {
		t.Fatalf("err = %v, want it accepted", err)
	}
	if len(ledger.Kept()) != 1 {
		t.Error("the feedback was lost with the mail")
	}
}

func TestOddFeedbackIsTurnedDown(t *testing.T) {
	srv, ledger := feedbackServer(t, 10, &postbox{})
	remote := synth.NewRemote(srv.URL, riya)

	for _, f := range []synth.Feedback{
		{Kind: "praise", Text: "nice"},
		{Kind: synth.FeedbackNote, Text: "   "},
		{Kind: synth.FeedbackNote, Text: strings.Repeat("a", synth.MostFeedbackBytes+1)},
	} {
		if err := remote.SendFeedback(context.Background(), f); !errors.Is(err, synth.ErrRefused) {
			t.Errorf("%q (%d bytes): err = %v, want refused", f.Kind, len(f.Text), err)
		}
	}
	if len(ledger.Kept()) != 0 {
		t.Errorf("kept %d", len(ledger.Kept()))
	}
}

func TestFeedbackCountsAgainstTheAllowance(t *testing.T) {
	srv, _ := feedbackServer(t, 1, &postbox{})
	remote := synth.NewRemote(srv.URL, riya)

	if err := remote.SendFeedback(context.Background(), synth.Feedback{Kind: synth.FeedbackNote, Text: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := remote.SendFeedback(context.Background(), synth.Feedback{Kind: synth.FeedbackNote, Text: "two"}); !errors.Is(err, synth.ErrOverCap) {
		t.Errorf("err = %v, want over the cap", err)
	}
}
