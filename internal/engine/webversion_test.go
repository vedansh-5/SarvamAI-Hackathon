package engine

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

type tabRecorder struct {
	frontCalls int
	ownCalls   int
	html       string
	frontErr   error
}

func (r *tabRecorder) Navigate(context.Context, string) error { return nil }

func (r *tabRecorder) URL(context.Context) (string, error) {
	return "https://web.whatsapp.com/", nil
}

func (r *tabRecorder) OuterHTML(context.Context) (string, error) { return r.html, nil }

func (r *tabRecorder) Front(context.Context) (string, string, error) {
	return "https://github.com/vedansh-5/VANS", r.html, nil
}

func (r *tabRecorder) Do(context.Context, string) error {
	r.frontCalls++
	return r.frontErr
}

func (r *tabRecorder) DoOnOwnTab(context.Context, string) error { r.ownCalls++; return nil }

func TestAnOwnTabPlanNeverTouchesTheTabInFront(t *testing.T) {
	rec := &tabRecorder{}
	e := &Engine{Browser: rec}

	plan := &Plan{Web: &WebPlan{URL: "https://web.whatsapp.com/", script: "noop", ownTab: true}}
	if _, err := e.runWeb(context.Background(), plan); err != nil {
		t.Fatalf("runWeb: %v", err)
	}
	if rec.frontCalls != 0 {
		t.Error("a plan for VANS's own tab reached the tab the user is reading")
	}
	if rec.ownCalls != 1 {
		t.Errorf("ran in the own tab %d time(s), want 1", rec.ownCalls)
	}
}

func TestAPageActionUsesTheTabInFront(t *testing.T) {
	rec := &tabRecorder{}
	e := &Engine{Browser: rec}

	plan := &Plan{Web: &WebPlan{URL: "https://github.com/", script: "noop"}}
	if _, err := e.runWeb(context.Background(), plan); err != nil {
		t.Fatalf("runWeb: %v", err)
	}
	if rec.frontCalls != 1 {
		t.Errorf("acted on the front tab %d time(s), want 1", rec.frontCalls)
	}
	if rec.ownCalls != 0 {
		t.Error("a page action was diverted into VANS's own tab")
	}
}

func TestWebVersionOfKnowsTheApplicationsWithoutTrees(t *testing.T) {
	cases := []struct {
		app  string
		want bool
	}{
		{"WhatsApp", true},
		{"whats app", true},
		{"Notion", true},
		{"Slack", true},
		{"Telegram", true},
		{"Notes", false},
		{"", false},
	}

	for _, c := range cases {
		got, ok := webVersionOf(c.app)
		if ok != c.want {
			t.Errorf("webVersionOf(%q) known = %v, want %v", c.app, ok, c.want)
		}
		if ok && got == "" {
			t.Errorf("webVersionOf(%q) returned an empty address", c.app)
		}
	}
}

func TestEveryWebVersionIsAUsableAddress(t *testing.T) {
	for app, target := range webVersions {
		if app != strings.ToLower(app) {
			t.Errorf("key %q is not the normalised form a lookup produces", app)
		}
		u, err := url.Parse(target)
		if err != nil {
			t.Errorf("%s: %q does not parse: %v", app, target, err)
			continue
		}
		if u.Scheme != "https" {
			t.Errorf("%s: %q is not https", app, target)
		}
		if u.Host == "" {
			t.Errorf("%s: %q names no host", app, target)
		}
	}
}
