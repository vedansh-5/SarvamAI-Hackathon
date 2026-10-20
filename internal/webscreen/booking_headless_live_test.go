//go:build integration

package webscreen

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/uiact"
)

type booking struct {
	Trip     string `json:"trip"`
	From     string `json:"from"`
	Date     string `json:"date"`
	Adults   int    `json:"adults"`
	Children int    `json:"children"`
	Rooms    int    `json:"rooms"`
	Searched bool   `json:"searched"`
}

func (h *tab) booked() booking {
	h.t.Helper()
	var b booking
	if err := json.Unmarshal([]byte(h.eval("document.getElementById('log').textContent")), &b); err != nil {
		h.t.Fatal(err)
	}
	return b
}

func (h *tab) named(s *Screen, role, start string) uiact.Selector {
	h.t.Helper()
	tree, err := s.Capture(h.ctx, "")
	if err != nil {
		h.t.Fatal(err)
	}
	for _, el := range tree.Elements {
		if el.Role == role && strings.HasPrefix(strings.ToLower(el.Title), strings.ToLower(start)) {
			return uiact.Selector{Role: role, Title: el.Title}
		}
	}
	h.t.Fatalf("no %s whose name starts %q: %s", role, start, titles(tree))
	return uiact.Selector{}
}

func openBooking(t *testing.T) (*tab, *Screen) {
	h := openHeadless(t)
	own, _ := servePages(t)
	if err := h.OpenInFront(h.ctx, own+"/booking.html"); err != nil {
		t.Fatal(err)
	}
	return h, h.screen()
}

func TestHeadlessABookingFormIsReadAsAPersonSeesIt(t *testing.T) {
	h, s := openBooking(t)

	h.named(s, "AXButton", "Close")
	h.named(s, "AXButton", "Round Trip")
	h.do(s, uiact.Step{Do: "click", Target: h.named(s, "AXButton", "One Way")})
	if got := h.booked().Trip; got != "one" {
		t.Errorf("trip is %q after clicking One Way, want one", got)
	}
	h.named(s, "AXDateField", "Departure")
	h.do(s, uiact.Step{Do: "click", Target: h.named(s, "AXLink", "SEARCH")})
	if !h.booked().Searched {
		t.Error("SEARCH, a link with no address, was not clicked")
	}

	h.do(s, uiact.Step{Do: "click", Target: h.named(s, "AXButton", "Travellers")})
	h.do(s, uiact.Step{Do: "click", Target: h.named(s, "AXButton", "ADULTS (12y +): 2")})
	if got := h.booked().Adults; got != 2 {
		t.Errorf("adults is %d after clicking 2 under ADULTS, want 2", got)
	}
	h.named(s, "AXButton", "CHILDREN (2y - 12y): 0")

	tree, err := s.Capture(h.ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, el := range tree.Elements {
		if strings.ContainsAny(el.Title, "\u200e\u200f\u200b\ufeff") {
			t.Errorf("%s %q carries an invisible mark", el.Role, el.Title)
		}
	}
}

func TestHeadlessACityBoxPicksTheSuggestionThatMatches(t *testing.T) {
	h, s := openBooking(t)
	from := h.named(s, "AXTextField", "From")

	h.do(s, uiact.Step{Do: "type", Target: from, Text: "Goa"})
	if got := h.booked().From; got != "Goa" {
		t.Errorf("From is %q after typing Goa, want Goa picked from the suggestions", got)
	}
	h.do(s, uiact.Step{Do: "type", Target: from, Text: "Mumbai"})
	if got := h.booked().From; got != "Mumbai" {
		t.Errorf("From is %q after typing Mumbai, want Mumbai rather than Navi Mumbai", got)
	}
}

func TestHeadlessADateIsPickedFromTheCalendar(t *testing.T) {
	h, s := openBooking(t)
	departure := h.named(s, "AXDateField", "Departure")

	h.do(s, uiact.Step{Do: "type", Target: departure, Text: "5 Dec 2026"})
	if got := h.booked().Date; got != "2026-12-05" {
		t.Errorf("the date is %q, want 2026-12-05, two months on", got)
	}
	h.do(s, uiact.Step{Do: "type", Target: h.named(s, "AXDateField", "Departure"), Text: "October 24, 2026"})
	if got := h.booked().Date; got != "2026-10-24" {
		t.Errorf("the date is %q, want 2026-10-24, back two months", got)
	}

	tree, err := s.Capture(h.ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	past := uiact.Step{Do: "type", Target: h.named(s, "AXDateField", "Departure"), Text: "10 Oct 2026"}
	err = s.Act(h.ctx, uiact.Program{App: tree.App, Steps: []uiact.Step{past}}, tree)
	if err == nil || !strings.Contains(err.Error(), "cannot be picked") {
		t.Errorf("a day the calendar greys out answered %v", err)
	}
}

func TestHeadlessACounterIsSetToANumber(t *testing.T) {
	h, s := openBooking(t)
	h.do(s, uiact.Step{Do: "click", Target: h.named(s, "AXButton", "Travellers")})
	time.Sleep(100 * time.Millisecond)
	rooms := h.named(s, "AXIncrementor", "Rooms")

	h.do(s, uiact.Step{Do: "set", Target: rooms, Text: "3"})
	if got := h.booked().Rooms; got != 3 {
		t.Errorf("rooms is %d, want 3", got)
	}
	h.do(s, uiact.Step{Do: "set", Target: rooms, Text: "2"})
	if got := h.booked().Rooms; got != 2 {
		t.Errorf("rooms is %d, want 2", got)
	}
}

func TestHeadlessTheBookingControlsArePutBackAsTheyWere(t *testing.T) {
	h, s := openBooking(t)
	keep := func(sel uiact.Selector) string {
		t.Helper()
		tree, err := s.Capture(h.ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		was, ok := s.ValueOf(h.ctx, tree, sel)
		if !ok {
			t.Fatalf("nothing kept of %s %q", sel.Role, sel.Title)
		}
		return was
	}

	from := h.named(s, "AXTextField", "From")
	wasFrom := keep(from)
	h.do(s, uiact.Step{Do: "type", Target: from, Text: "Goa"})

	h.do(s, uiact.Step{Do: "type", Target: h.named(s, "AXDateField", "Departure"), Text: "24 Oct 2026"})
	departure := h.named(s, "AXDateField", "Departure")
	wasDate := keep(departure)
	h.do(s, uiact.Step{Do: "type", Target: departure, Text: "5 Dec 2026"})

	h.do(s, uiact.Step{Do: "click", Target: h.named(s, "AXButton", "Travellers")})
	time.Sleep(100 * time.Millisecond)
	rooms := h.named(s, "AXIncrementor", "Rooms")
	wasRooms := keep(rooms)
	h.do(s, uiact.Step{Do: "set", Target: rooms, Text: "3"})

	if err := s.PutBack(h.ctx, "", rooms, wasRooms); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBack(h.ctx, "", h.named(s, "AXDateField", "Departure"), wasDate); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBack(h.ctx, "", from, wasFrom); err != nil {
		t.Fatal(err)
	}
	got := h.booked()
	if got.From != "Delhi" || got.Date != "2026-10-24" || got.Rooms != 1 {
		t.Errorf("after undo the form holds %+v, want From Delhi, 24 Oct and 1 room again", got)
	}
}
