package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
)

func TestATesterGetsTheVersionChosenForThemWithoutItCounting(t *testing.T) {
	runVoiceTrial(t)
	srv, ledger := feedbackServer(t, 1, &postbox{})
	remote := synth.NewRemote(srv.URL, riya)

	got, err := remote.Trials(context.Background())
	if err != nil || got["voice"] != "off" {
		t.Fatalf("before any choice got %v, %v; want the first version", got, err)
	}
	if err := ledger.SetTrial(context.Background(), riya, "voice", "on"); err != nil {
		t.Fatal(err)
	}
	if got, _ = remote.Trials(context.Background()); got["voice"] != "on" {
		t.Errorf("after choosing on got %v", got)
	}
	if err := remote.SendFeedback(context.Background(), synth.Feedback{Kind: synth.FeedbackNote, Text: "talking is quicker"}); err != nil {
		t.Fatalf("asking for trials used up the allowance: %v", err)
	}
	kept := ledger.Kept()
	if len(kept) != 1 || kept[0].Trials != "voice=on" {
		t.Errorf("feedback kept as %+v; want it tagged with the version they saw", kept)
	}
}

func TestATrialThatDoesNotExistIsTurnedDown(t *testing.T) {
	runVoiceTrial(t)
	ledger := NewMemory([]Tester{{Name: "Riya", Code: riya}})
	for _, pair := range [][2]string{{"teleport", "on"}, {"voice", "loud"}} {
		if err := ledger.SetTrial(context.Background(), riya, pair[0], pair[1]); err == nil {
			t.Errorf("%s=%s was accepted", pair[0], pair[1])
		}
	}
}

func TestTheConsoleSetsATrialAndComparesTheVersions(t *testing.T) {
	runVoiceTrial(t)
	srv, ledger, client := consoleServer(t)
	signIn(t, srv, client)

	page(t, client, http.MethodPost, srv.URL+"/console/testers/trial", url.Values{"code": {riya}, "trial": {"voice"}, "variant": {"on"}, "back": {"Riya"}})
	who, _, _ := ledger.Tester(context.Background(), riya)
	if synth.TrialsFor(who.Trials)["voice"] != "on" {
		t.Fatalf("trials are %v after choosing on", who.Trials)
	}
	_ = ledger.Keep(context.Background(), Feedback{From: "Riya", Kind: synth.FeedbackNote, Text: "talking is quicker", At: time.Now(), Trials: "voice=on"})

	_, body := page(t, client, http.MethodGet, srv.URL+"/console/trials", nil)
	newCard := body[strings.Index(body, "<h2>on"):]
	if !strings.Contains(newCard, "Riya") || !strings.Contains(newCard, "talking is quicker") {
		t.Errorf("the on version's card does not show Riya and her feedback:\n%s", body)
	}
	_, body = page(t, client, http.MethodGet, srv.URL+"/console/testers/Riya", nil)
	if !strings.Contains(body, `<option value="on" selected>`) {
		t.Errorf("the tester page does not show on as chosen")
	}
}

func runVoiceTrial(t *testing.T) {
	t.Cleanup(synth.RunTrials([]synth.Trial{{Name: "voice", Says: "Voice", Variants: []string{"off", "on"}}}))
}

func TestWithNoTrialRunningEveryoneGetsNone(t *testing.T) {
	srv, _ := feedbackServer(t, 1, &postbox{})
	got, err := synth.NewRemote(srv.URL, riya).Trials(context.Background())
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want no trials", got, err)
	}
}
