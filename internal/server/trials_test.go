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
	srv, ledger := feedbackServer(t, 1, &postbox{})
	remote := synth.NewRemote(srv.URL, riya)

	got, err := remote.Trials(context.Background())
	if err != nil || got[synth.TrialSetup] != synth.SetupOld {
		t.Fatalf("before any choice got %v, %v; want the first version", got, err)
	}
	if err := ledger.SetTrial(context.Background(), riya, synth.TrialSetup, synth.SetupNew); err != nil {
		t.Fatal(err)
	}
	if got, _ = remote.Trials(context.Background()); got[synth.TrialSetup] != synth.SetupNew {
		t.Errorf("after choosing new got %v", got)
	}
	if err := remote.SendFeedback(context.Background(), synth.Feedback{Kind: synth.FeedbackNote, Text: "the new Setup is shorter"}); err != nil {
		t.Fatalf("asking for trials used up the allowance: %v", err)
	}
	kept := ledger.Kept()
	if len(kept) != 1 || kept[0].Trials != "setup=new" {
		t.Errorf("feedback kept as %+v; want it tagged with the version they saw", kept)
	}
}

func TestATrialThatDoesNotExistIsTurnedDown(t *testing.T) {
	ledger := NewMemory([]Tester{{Name: "Riya", Code: riya}})
	for _, pair := range [][2]string{{"voice", "on"}, {synth.TrialSetup, "newest"}} {
		if err := ledger.SetTrial(context.Background(), riya, pair[0], pair[1]); err == nil {
			t.Errorf("%s=%s was accepted", pair[0], pair[1])
		}
	}
}

func TestTheConsoleSetsATrialAndComparesTheVersions(t *testing.T) {
	srv, ledger, client := consoleServer(t)
	signIn(t, srv, client)

	page(t, client, http.MethodPost, srv.URL+"/console/testers/trial", url.Values{"code": {riya}, "trial": {"setup"}, "variant": {"new"}, "back": {"Riya"}})
	who, _, _ := ledger.Tester(context.Background(), riya)
	if synth.TrialsFor(who.Trials)[synth.TrialSetup] != synth.SetupNew {
		t.Fatalf("trials are %v after choosing new", who.Trials)
	}
	_ = ledger.Keep(context.Background(), Feedback{From: "Riya", Kind: synth.FeedbackNote, Text: "groups help", At: time.Now(), Trials: "setup=new"})

	_, body := page(t, client, http.MethodGet, srv.URL+"/console/trials", nil)
	newCard := body[strings.Index(body, "<h2>new"):]
	if !strings.Contains(newCard, "Riya") || !strings.Contains(newCard, "groups help") {
		t.Errorf("the new version's card does not show Riya and her feedback:\n%s", body)
	}
	_, body = page(t, client, http.MethodGet, srv.URL+"/console/testers/Riya", nil)
	if !strings.Contains(body, `<option value="new" selected>`) {
		t.Errorf("the tester page does not show new as chosen")
	}
}
