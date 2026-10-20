package webscreen

import (
	"testing"
	"time"
)

func TestDatesAreReadInTheShapesPagesAndPeopleWriteThem(t *testing.T) {
	for text, want := range map[string]string{
		"5 Dec 2026":                       "2026-12-05",
		"October 24, 2026":                 "2026-10-24",
		"Thu Oct 01 2026":                  "2026-10-01",
		"Choose Saturday, March 7th, 2027": "2027-03-07",
		"2026-11-30":                       "2026-11-30",
		"24/10/2026":                       "2026-10-24",
		"Departure 20 Oct26 Tuesday":       "2026-10-20",
		"Sept 3 2026":                      "2026-09-03",
	} {
		got, ok := dateIn(text, 2026)
		if !ok || got.Format("2006-01-02") != want {
			t.Errorf("dateIn(%q) = %v, %v; want %s", text, got.Format("2006-01-02"), ok, want)
		}
	}
	for _, text := range []string{"Mon", "Sunday flights", "19 9,622", "31 Feb 2026", "Next Month", "ADULTS (12y +): 2"} {
		if got, ok := dateIn(text, 2026); ok {
			t.Errorf("dateIn(%q) = %s, want no date", text, got.Format("2006-01-02"))
		}
	}
}

func TestADateWithoutAYearIsTheNextOneToCome(t *testing.T) {
	today := time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)
	for text, want := range map[string]string{
		"24 Oct":     "2026-10-24",
		"19 Oct":     "2026-10-19",
		"3 Jan":      "2027-01-03",
		"3 Jan 2026": "2026-01-03",
	} {
		got, ok := wantedDate(text, today)
		if !ok || got.Format("2006-01-02") != want {
			t.Errorf("wantedDate(%q) = %s, %v; want %s", text, got.Format("2006-01-02"), ok, want)
		}
	}
}

func TestTheSuggestionThatFitsBestIsTheOneThatStartsWithWhatWasTyped(t *testing.T) {
	typed := "Mumbai"
	best, pick := 0, ""
	for _, option := range []string{"Navi Mumbai, India NMI", "Mumbai, India BOM Chhatrapati Shivaji", "Mumbai Central"} {
		if score := fits(typed, option); score > best {
			best, pick = score, option
		}
	}
	if pick != "Mumbai, India BOM Chhatrapati Shivaji" {
		t.Errorf("picked %q", pick)
	}
	if fits("Goa", "Chicago") != 0 {
		t.Error("Goa should not fit Chicago")
	}
	if fits("goa", "Mopa, Goa, India") == 0 {
		t.Error("Goa should fit Mopa, Goa as a word")
	}
}
