//go:build integration

package engine

import (
	"encoding/json"
	"testing"

	"github.com/vedansh/vans/internal/headless"
	"github.com/vedansh/vans/internal/webscreen"
)

type bookingState struct {
	Trip     string `json:"trip"`
	From     string `json:"from"`
	To       string `json:"to"`
	Date     string `json:"date"`
	Adults   int    `json:"adults"`
	Rooms    int    `json:"rooms"`
	Searched bool   `json:"searched"`
}

func TestLiveABookingFormIsFilledWithItsSuggestionsCalendarAndCounts(t *testing.T) {
	steps, plans := liveModel(t)
	c := headless.Open(t)
	site, _ := headless.Serve(t, "../webscreen/testdata")
	if err := c.OpenInFront(c.Ctx, site+"/booking.html"); err != nil {
		t.Fatal(err)
	}
	screen := &webscreen.Screen{Page: c, Browser: "Google Chrome", RealDrag: c.Drag, RealClick: c.Click, RealType: c.TypeText, RealMove: c.Move}
	tree, err := screen.Capture(c.Ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	said := &lines{}
	m := &memoryMap{}
	e := &Engine{UIScreen: screen, Steps: steps, Plans: plans, Map: m, Tasks: m, Log: said.logf}
	task := "fill in a one way trip from Goa to Mumbai on 5 Dec 2026 for 2 adults and 2 rooms"
	plan, err := e.driveOn(c.Ctx, tree.App, task, task, nil)
	if err != nil {
		t.Fatalf("%s: %v", task, err)
	}
	_, err = e.runDrive(c.Ctx, plan)
	t.Logf("%s:\n%s", task, said.all())
	if err != nil {
		t.Fatal(err)
	}

	var got bookingState
	if err := json.Unmarshal([]byte(c.Eval("document.getElementById('log').textContent")), &got); err != nil {
		t.Fatal(err)
	}
	want := bookingState{Trip: "one", From: "Goa", To: "Mumbai", Date: "2026-12-05", Adults: 2, Rooms: 2}
	if got != want {
		t.Errorf("the form holds %+v, want %+v", got, want)
	}
	if len(m.tasks) == 1 {
		t.Logf("%d model calls for %d steps", m.tasks[0].Calls, m.tasks[0].Steps)
	}
}
