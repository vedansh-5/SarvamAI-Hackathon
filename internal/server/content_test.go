package server

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
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

func TestTheConsoleSavesOnlyWhatWasChanged(t *testing.T) {
	srv, ledger, client := consoleServer(t)
	signIn(t, srv, client)

	_, body := page(t, client, http.MethodGet, srv.URL+"/console/content", nil)
	if !strings.Contains(body, "Nothing is saved") || !strings.Contains(body, `value="Show you how"`) {
		t.Errorf("with nothing saved, the boxes do not start from the built-in wording:\n%s", body)
	}

	form := wordingValues(wordingFrom(synth.Content{}))
	page(t, client, http.MethodPost, srv.URL+"/console/content/save", form)
	if raw, _ := ledger.Content(context.Background()); raw != "" {
		t.Errorf("saving the built-in wording unchanged kept %q", raw)
	}

	form.Set("title", "Hello")
	form.Set("a.later.order", "0")
	form.Set("a.files.hidden", "yes")
	form.Set("a.guide.examples", "how do I crop\n\n  how do I rotate  ")
	_, body = page(t, client, http.MethodPost, srv.URL+"/console/content/save", form)
	raw, _ := ledger.Content(context.Background())
	got, err := synth.ParseContent(raw)
	if err != nil || !strings.Contains(body, "Saved.") {
		t.Fatalf("kept %q, %v", raw, err)
	}
	if got.Welcome == nil || got.Welcome.Title != "Hello" || got.Welcome.Lede != "" || got.Welcome.Keys != nil {
		t.Errorf("welcome kept as %+v; want only the title", got.Welcome)
	}
	if len(got.Abilities) == 0 || got.Abilities[0].ID != synth.AbilityLater || got.Abilities[0].Title != "" {
		t.Errorf("abilities kept as %+v; want Do it later first, its words left to the build", got.Abilities)
	}
	shown := got.AbilitiesOver(synth.BuiltInAbilities())
	for _, a := range shown {
		if a.ID == synth.AbilityFiles {
			t.Error("a hidden ability is still listed")
		}
		if a.ID == synth.AbilityGuide && strings.Join(a.Examples, "|") != "how do I crop|how do I rotate" {
			t.Errorf("examples kept as %q", a.Examples)
		}
	}

	_, body = page(t, client, http.MethodGet, srv.URL+"/console/content", nil)
	if !strings.Contains(body, `value="Hello"`) || !strings.Contains(body, `name="a.files.hidden" value="yes" checked`) {
		t.Errorf("the page does not show what was saved:\n%s", body)
	}

	broken := wordingValues(wordingFrom(got))
	broken["key.before"][3] = "Press"
	status, body := page(t, client, http.MethodPost, srv.URL+"/console/content/save", broken)
	if status != http.StatusOK || !strings.Contains(body, "Not saved") || !strings.Contains(body, `value="Press"`) {
		t.Errorf("a key line with no key answered %d:\n%s", status, body)
	}
	if after, _ := ledger.Content(context.Background()); after != raw {
		t.Errorf("a refusal changed what was kept to %q", after)
	}

	page(t, client, http.MethodPost, srv.URL+"/console/content/clear", url.Values{})
	if raw, _ := ledger.Content(context.Background()); raw != "" {
		t.Errorf("after going back to the built-in wording, kept %q", raw)
	}
}

func wordingValues(f wordingForm) url.Values {
	v := url.Values{"title": {f.Title}, "lede": {f.Lede}}
	for _, k := range f.Keys {
		v.Add("key.before", k.Before)
		v.Add("key.key", k.Key)
		v.Add("key.after", k.After)
	}
	for _, a := range f.Abilities {
		p := "a." + a.ID + "."
		v.Set(p+"order", strconv.Itoa(a.Order))
		v.Set(p+"title", a.Title)
		v.Set(p+"says", a.Says)
		v.Set(p+"examples", a.Examples)
		if a.Hidden {
			v.Set(p+"hidden", "yes")
		}
	}
	return v
}
