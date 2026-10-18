package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
)

func TestTheWordingIsSentWithoutCountingIt(t *testing.T) {
	srv, ledger := feedbackServer(t, 1, &postbox{})
	remote := synth.NewRemote(srv.URL, riya)

	got, err := remote.Content(context.Background())
	if err != nil || got.Welcome != nil || len(got.Abilities) != 0 {
		t.Fatalf("with nothing saved got %+v, %v; want nothing, so the app keeps its own", got, err)
	}

	_ = ledger.SetContent(context.Background(), `{"welcome":{"title":"Hello"},"abilities":[{"id":"files","hidden":true}]}`)
	for i := 0; i < 3; i++ {
		if got, err = remote.Content(context.Background()); err != nil {
			t.Fatalf("asking for the wording a %dth time: %v", i+1, err)
		}
	}
	if got.Welcome == nil || got.Welcome.Title != "Hello" || len(got.Abilities) != 1 || !got.Abilities[0].Hidden {
		t.Errorf("got %+v", got)
	}
	if err := remote.SendFeedback(context.Background(), synth.Feedback{Kind: synth.FeedbackNote, Text: "hi"}); err != nil {
		t.Errorf("a daily allowance of one was used up by asking for the wording: %v", err)
	}
}

func TestBrokenWordingOnTheServerIsNotPassedOn(t *testing.T) {
	srv, ledger := feedbackServer(t, 10, &postbox{})
	_ = ledger.SetContent(context.Background(), `{"welcome":`)
	got, err := synth.NewRemote(srv.URL, riya).Content(context.Background())
	if err != nil || got.Welcome != nil {
		t.Errorf("got %+v, %v; want nothing", got, err)
	}
}

func TestTheConsoleSavesOnlyWordingThatReads(t *testing.T) {
	srv, ledger, client := consoleServer(t)
	signIn(t, srv, client)

	_, body := page(t, client, http.MethodGet, srv.URL+"/console/content", nil)
	if !strings.Contains(body, "Nothing is saved") || !strings.Contains(body, "Show you how") {
		t.Errorf("with nothing saved, the page does not start from the built-in wording:\n%s", body)
	}

	status, body := page(t, client, http.MethodPost, srv.URL+"/console/content/save", url.Values{"doc": {`{"abilities":[{"id":"teleport"}]}`}})
	if status != http.StatusOK || !strings.Contains(body, "Not saved") || !strings.Contains(body, "teleport") {
		t.Errorf("an unknown ability answered %d:\n%s", status, body)
	}
	if raw, _ := ledger.Content(context.Background()); raw != "" {
		t.Errorf("kept %q after a refusal", raw)
	}

	doc := `{"welcome":{"title":"Hello"}}`
	_, body = page(t, client, http.MethodPost, srv.URL+"/console/content/save", url.Values{"doc": {doc}})
	if raw, _ := ledger.Content(context.Background()); raw != doc || !strings.Contains(body, "Saved.") {
		t.Errorf("kept %q, want %q", raw, doc)
	}

	page(t, client, http.MethodPost, srv.URL+"/console/content/clear", url.Values{})
	if raw, _ := ledger.Content(context.Background()); raw != "" {
		t.Errorf("after going back to the built-in wording, kept %q", raw)
	}
}
