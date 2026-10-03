package when

import (
	"testing"
	"time"
)

var friday = time.Date(2026, 10, 2, 16, 30, 0, 0, time.Local)

func TestWhenAndWhatAreToldApart(t *testing.T) {
	for _, tc := range []struct {
		said, rest, rule string
	}{
		{`every weekday at 9am run "morning report"`, `run "morning report"`, "every weekday at 9 am"},
		{`every monday at 9:30 check my mail`, `check my mail`, "every Monday at 9:30 am"},
		{`every day at 18:00: top stories on hn`, `: top stories on hn`, "every day at 6 pm"},
		{`every evening tidy the desktop`, `tidy the desktop`, "every day at 6 pm"},
		{`every weekend at noon water the plants`, `water the plants`, "at weekends at 12 pm"},
		{`schedule "standup notes" every tue at 10`, `schedule "standup notes"`, "every Tuesday at 10 am"},
		{`at 5 remind me to call the dentist`, `remind me to call the dentist`, "once, on Fri 2 Oct at 5 pm"},
		{`at 9am send the report`, `send the report`, "once, on Sat 3 Oct at 9 am"},
		{`tomorrow at 8am back up my notes`, `back up my notes`, "once, on Sat 3 Oct at 8 am"},
		{`in 20 minutes remind me to stretch`, `remind me to stretch`, "once, on Fri 2 Oct at 4:50 pm"},
		{`in an hour check the build`, `check the build`, "once, on Fri 2 Oct at 5:30 pm"},
	} {
		rule, rest, ok := Parse(tc.said, friday)
		if !ok {
			t.Errorf("%q: no time was found in it", tc.said)
			continue
		}
		if rest != tc.rest || rule.String() != tc.rule {
			t.Errorf("%q:\n  got  %q, %q\n  want %q, %q", tc.said, rest, rule.String(), tc.rest, tc.rule)
		}
	}
}

func TestWhatSaysNoTimeIsLeftAlone(t *testing.T) {
	for _, said := range []string{
		"send hi to Joe on whatsapp",
		"every tab should be closed",
		"how do I look at the calendar",
		"today at 9am do the thing",
		"at 25:00 do the thing",
	} {
		if rule, _, ok := Parse(said, friday); ok {
			t.Errorf("%q was read as %s", said, rule)
		}
	}
}

func TestTheNextTimeIsWorkedOutFromTheRule(t *testing.T) {
	at := func(day, hour, min int) time.Time { return time.Date(2026, 10, day, hour, min, 0, 0, time.Local) }

	for _, tc := range []struct {
		rule Rule
		want time.Time
	}{
		{Rule{Kind: Daily, Minute: 9 * 60}, at(3, 9, 0)},
		{Rule{Kind: Daily, Minute: 17 * 60}, at(2, 17, 0)},
		{Rule{Kind: Weekdays, Minute: 9 * 60}, at(5, 9, 0)},
		{Rule{Kind: Weekends, Minute: 9 * 60}, at(3, 9, 0)},
		{Rule{Kind: Weekly, Weekday: time.Friday, Minute: 9 * 60}, at(9, 9, 0)},
		{Rule{Kind: Weekly, Weekday: time.Friday, Minute: 17 * 60}, at(2, 17, 0)},
	} {
		got, ok := tc.rule.Next(friday)
		if !ok || !got.Equal(tc.want) {
			t.Errorf("%s after Friday 4:30 pm: got %v, %v; want %v", tc.rule, got, ok, tc.want)
		}
	}

	past := Rule{Kind: Once, At: friday.Add(-time.Minute).Unix()}
	if _, ok := past.Next(friday); ok {
		t.Error("a one-off whose time has gone was given another time")
	}
}
