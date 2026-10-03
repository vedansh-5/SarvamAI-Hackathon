package when

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	Once     = "once"
	Daily    = "daily"
	Weekdays = "weekdays"
	Weekends = "weekends"
	Weekly   = "weekly"
)

type Rule struct {
	Kind    string       `json:"kind"`
	Weekday time.Weekday `json:"weekday,omitempty"`
	Minute  int          `json:"minute"`
	At      int64        `json:"at,omitempty"`
}

const (
	clock = `(\d{1,2})(?:[:.](\d{2}))?\s*(a\.?m\.?|p\.?m\.?)?|noon|midnight`
	days  = `day|weekday|weekend|morning|evening|night|monday|tuesday|wednesday|thursday|friday|saturday|sunday|mon|tue|tues|wed|thu|thur|thurs|fri|sat|sun`
)

var (
	repeating = regexp.MustCompile(`(?i)\bevery\s+(` + days + `)s?\b(?:\s+(?:at|@)\s+(` + clock + `))?`)
	onADay    = regexp.MustCompile(`(?i)\b(today|tomorrow)\s+(?:at|@)\s+(` + clock + `)`)
	atATime   = regexp.MustCompile(`(?i)\b(?:at|@)\s+(` + clock + `)`)
	inAWhile  = regexp.MustCompile(`(?i)\bin\s+(\d{1,3}|an?)\s*(minutes?|mins?|hours?|hrs?)\b`)
	aClock    = regexp.MustCompile(`(?i)^(?:` + clock + `)$`)
)

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

const (
	morning = 9 * 60
	evening = 18 * 60
	night   = 21 * 60
)

func minuteOf(text string) (int, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	switch text {
	case "":
		return 0, false
	case "noon":
		return 12 * 60, true
	case "midnight":
		return 0, true
	}
	m := aClock.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	hour, _ := strconv.Atoi(m[1])
	minute := 0
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}
	half := strings.ReplaceAll(m[3], ".", "")
	switch {
	case minute > 59 || hour > 23:
		return 0, false
	case half == "am":
		if hour < 1 || hour > 12 {
			return 0, false
		}
		hour %= 12
	case half == "pm":
		if hour < 1 || hour > 12 {
			return 0, false
		}
		hour = hour%12 + 12
	case m[2] == "" && hour >= 1 && hour <= 6:
		hour += 12
	}
	return hour*60 + minute, true
}

func Parse(text string, now time.Time) (Rule, string, bool) {
	cut := func(loc []int) string {
		rest := text[:loc[0]] + " " + text[loc[1]:]
		return strings.Join(strings.Fields(rest), " ")
	}

	if loc := repeating.FindStringSubmatchIndex(text); loc != nil {
		m := repeating.FindStringSubmatch(text)
		day := strings.ToLower(m[1])
		rule := Rule{Minute: morning}
		switch day {
		case "day":
			rule.Kind = Daily
		case "morning":
			rule.Kind = Daily
		case "evening":
			rule.Kind, rule.Minute = Daily, evening
		case "night":
			rule.Kind, rule.Minute = Daily, night
		case "weekday":
			rule.Kind = Weekdays
		case "weekend":
			rule.Kind = Weekends
		default:
			rule.Kind, rule.Weekday = Weekly, weekdays[day[:3]]
		}
		if m[2] != "" {
			minute, ok := minuteOf(m[2])
			if !ok {
				return Rule{}, text, false
			}
			rule.Minute = minute
		}
		return rule, cut(loc), true
	}

	if loc := inAWhile.FindStringSubmatchIndex(text); loc != nil {
		m := inAWhile.FindStringSubmatch(text)
		n := 1
		if v, err := strconv.Atoi(m[1]); err == nil {
			n = v
		}
		unit := time.Minute
		if strings.HasPrefix(strings.ToLower(m[2]), "h") {
			unit = time.Hour
		}
		if n < 1 {
			return Rule{}, text, false
		}
		return Rule{Kind: Once, At: now.Add(time.Duration(n) * unit).Unix()}, cut(loc), true
	}

	if loc := onADay.FindStringSubmatchIndex(text); loc != nil {
		m := onADay.FindStringSubmatch(text)
		minute, ok := minuteOf(m[2])
		if !ok {
			return Rule{}, text, false
		}
		at := atMinute(now, minute)
		if strings.EqualFold(m[1], "tomorrow") {
			at = at.AddDate(0, 0, 1)
		} else if !at.After(now) {
			return Rule{}, text, false
		}
		return Rule{Kind: Once, Minute: minute, At: at.Unix()}, cut(loc), true
	}

	if loc := atATime.FindStringSubmatchIndex(text); loc != nil {
		m := atATime.FindStringSubmatch(text)
		minute, ok := minuteOf(m[1])
		if !ok {
			return Rule{}, text, false
		}
		at := atMinute(now, minute)
		if !at.After(now) {
			at = at.AddDate(0, 0, 1)
		}
		return Rule{Kind: Once, Minute: minute, At: at.Unix()}, cut(loc), true
	}

	return Rule{}, text, false
}

func atMinute(day time.Time, minute int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), minute/60, minute%60, 0, 0, day.Location())
}

func (r Rule) fits(day time.Weekday) bool {
	switch r.Kind {
	case Daily:
		return true
	case Weekdays:
		return day != time.Saturday && day != time.Sunday
	case Weekends:
		return day == time.Saturday || day == time.Sunday
	case Weekly:
		return day == r.Weekday
	}
	return false
}

func (r Rule) Next(after time.Time) (time.Time, bool) {
	if r.Kind == Once {
		at := time.Unix(r.At, 0)
		return at, at.After(after)
	}
	for add := 0; add <= 7; add++ {
		candidate := atMinute(after.AddDate(0, 0, add), r.Minute)
		if candidate.After(after) && r.fits(candidate.Weekday()) {
			return candidate, true
		}
	}
	return time.Time{}, false
}

func clockOf(minute int) string {
	hour, min := minute/60, minute%60
	half := "am"
	if hour >= 12 {
		half = "pm"
	}
	hour %= 12
	if hour == 0 {
		hour = 12
	}
	if min == 0 {
		return fmt.Sprintf("%d %s", hour, half)
	}
	return fmt.Sprintf("%d:%02d %s", hour, min, half)
}

func (r Rule) String() string {
	switch r.Kind {
	case Daily:
		return "every day at " + clockOf(r.Minute)
	case Weekdays:
		return "every weekday at " + clockOf(r.Minute)
	case Weekends:
		return "at weekends at " + clockOf(r.Minute)
	case Weekly:
		return fmt.Sprintf("every %s at %s", r.Weekday, clockOf(r.Minute))
	case Once:
		at := time.Unix(r.At, 0)
		return "once, on " + at.Format("Mon 2 Jan") + " at " + clockOf(at.Hour()*60+at.Minute())
	}
	return "never"
}
