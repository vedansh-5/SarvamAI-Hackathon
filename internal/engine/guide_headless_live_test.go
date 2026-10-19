//go:build integration

package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/headless"
	"github.com/vedansh/vans/internal/webscreen"
)

type signUp struct {
	Name  string `json:"name"`
	City  string `json:"city"`
	Email string `json:"email"`
}

func readSignUp(t *testing.T, c *headless.Chrome) signUp {
	t.Helper()
	var f signUp
	raw := c.Eval("JSON.stringify({name: document.getElementById('name').value, city: document.getElementById('city').value, email: document.getElementById('email').value})")
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestLiveYesToAllFillsTheRestOfAFormAndCanBePutBack(t *testing.T) {
	steps, plans := liveModel(t)
	wait := guideStepTimeout
	guideStepTimeout = 30 * time.Second
	t.Cleanup(func() { guideStepTimeout = wait })
	c := headless.Open(t)
	site, _ := headless.Serve(t, "testdata/app")
	if err := c.OpenInFront(c.Ctx, site+"/form.html"); err != nil {
		t.Fatal(err)
	}
	screen := &webscreen.Screen{Page: c, Browser: "Google Chrome", RealDrag: c.Drag}
	said := &lines{}
	deeds := &recordedDeeds{}
	e := &Engine{UIScreen: screen, PageScreen: screen, Steps: steps, Plans: loggedPlans{t, plans}, Deeds: deeds, Log: said.logf}
	reply := offering(e)

	offers, all := 0, 0
	task := "fill in the form: name Ada Lovelace, city London, email ada@example.com"
	_, err := e.RunGuide(c.Ctx, &Plan{Guide: &GuidePlan{App: "Google Chrome", Web: true, Browser: "Google Chrome", Task: task}}, func(s GuideStep) {
		if s.Offer {
			offers++
			if s.OfferAll {
				all++
			}
			answering(reply, OfferTakenForAll)
		}
	}, nil)
	t.Logf("what the engine said:\n%s", said.all())
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}

	got := readSignUp(t, c)
	if got != (signUp{Name: "Ada Lovelace", City: "London", Email: "ada@example.com"}) {
		t.Errorf("the form holds %+v", got)
	}
	if offers != 1 || all != 1 {
		t.Errorf("offered %d time(s), %d with yes to all; want one offer, and nothing asked after yes to all", offers, all)
	}
	if len(deeds.deeds) != 1 || !strings.Contains(deeds.deeds[0].Undo, undoByPuttingTextBack) {
		t.Fatalf("recorded %+v, want one deed that can put the text back", deeds.deeds)
	}

	t.Logf("undo: %s", deeds.deeds[0].Undo)
	var how howToUndo
	if err := json.Unmarshal([]byte(deeds.deeds[0].Undo), &how); err != nil {
		t.Fatal(err)
	}
	if err := e.putItBack(context.Background(), how); err != nil {
		t.Fatalf("putItBack: %v", err)
	}
	if got := readSignUp(t, c); got != (signUp{}) {
		t.Errorf("after putting it back the form holds %+v, want it empty again", got)
	}
}
