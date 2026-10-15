package engine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/store"
)

const (
	AtATime     = "time"
	WhenOpened  = "opened"
	AfterAnApp  = "after"
	BeforeDone  = "finish"
	daysToCount = 3
	nearTheTime = 20
	afterOpened = 10 * time.Minute
	afterTheApp = 5 * time.Minute
	mostOften   = 0.6
	mostTimed   = 5
	beforeDone  = 15 * time.Minute
	offerEarly  = 10
)

type Trigger struct {
	Kind     string `json:"kind"`
	Minute   int    `json:"minute,omitempty"`
	Weekdays bool   `json:"weekdays,omitempty"`
	App      string `json:"app,omitempty"`
}

type Timed struct {
	Key     string      `json:"key"`
	Trigger Trigger     `json:"trigger"`
	Action  store.Click `json:"action"`
	Days    int         `json:"days"`
}

func dayOf(t time.Time) string { return t.Format("2006-01-02") }

func minuteOf(t time.Time) int { return t.Hour()*60 + t.Minute() }

func isWeekday(day string) bool {
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return false
	}
	return t.Weekday() != time.Saturday && t.Weekday() != time.Sunday
}

func actionKey(c store.Click) string {
	if c.Role == store.RoleFront {
		return c.App + "\x1f" + store.RoleFront
	}
	return clickKey(c)
}

func (t Timed) key() string {
	return fmt.Sprintf("%s|%d|%v|%s|%s", t.Trigger.Kind, t.Trigger.Minute, t.Trigger.Weekdays, t.Trigger.App, actionKey(t.Action))
}

func FindTimed(events []store.Click) []Timed {
	type firsts map[string]time.Time
	byDay := map[string][]store.Click{}
	var days []string
	for _, e := range events {
		d := dayOf(e.At)
		if _, ok := byDay[d]; !ok {
			days = append(days, d)
		}
		byDay[d] = append(byDay[d], e)
	}
	sort.Strings(days)

	firstOf := map[string]firsts{}
	lastOf := map[string]firsts{}
	actions := map[string]store.Click{}
	opened := map[string]time.Time{}
	closed := map[string]time.Time{}
	appFirst := map[string]firsts{}
	for _, d := range days {
		for _, e := range byDay[d] {
			if e.Role == store.RoleMoment {
				if (e.Title == "unlock" || e.Title == "wake") && opened[d].IsZero() {
					opened[d] = e.At
				}
				if (e.Title == "lock" || e.Title == "sleep") && e.At.After(closed[d]) {
					closed[d] = e.At
				}
				continue
			}
			k := actionKey(e)
			if firstOf[k] == nil {
				firstOf[k] = firsts{}
				actions[k] = store.Click{App: e.App, Role: e.Role, Title: e.Title}
			}
			if _, ok := firstOf[k][d]; !ok {
				firstOf[k][d] = e.At
			}
			if lastOf[k] == nil {
				lastOf[k] = firsts{}
			}
			if e.At.After(lastOf[k][d]) {
				lastOf[k][d] = e.At
			}
			if e.Role == store.RoleFront {
				if appFirst[e.App] == nil {
					appFirst[e.App] = firsts{}
				}
				if _, ok := appFirst[e.App][d]; !ok {
					appFirst[e.App][d] = e.At
				}
			}
		}
	}

	best := map[string]Timed{}
	consider := func(t Timed) {
		k := actionKey(t.Action)
		if old, ok := best[k]; ok && (old.Days > t.Days || old.Days == t.Days && t.Trigger.Kind != BeforeDone) {
			return
		}
		best[k] = t
	}

	for k, seen := range firstOf {
		action := actions[k]
		if len(seen) >= daysToCount {
			if t, ok := atATime(seen); ok {
				t.Action = action
				consider(t)
			}
		}

		n := 0
		for d, at := range seen {
			o := opened[d]
			if !o.IsZero() && !at.Before(o) && at.Sub(o) <= afterOpened {
				n++
			}
		}
		if n >= daysToCount {
			consider(Timed{Trigger: Trigger{Kind: WhenOpened}, Action: action, Days: n})
		}

		var ends []time.Time
		var endDays []string
		for d, at := range lastOf[k] {
			c := closed[d]
			if !c.IsZero() && !at.After(c) && c.Sub(at) <= beforeDone && lastActivity(byDay[d], c) {
				ends = append(ends, at)
				endDays = append(endDays, d)
			}
		}
		if len(ends) >= daysToCount {
			sort.Slice(ends, func(i, j int) bool { return minuteOf(ends[i]) < minuteOf(ends[j]) })
			weekdays := true
			for _, d := range endDays {
				weekdays = weekdays && isWeekday(d)
			}
			consider(Timed{Trigger: Trigger{Kind: BeforeDone, Minute: minuteOf(ends[len(ends)/2]), Weekdays: weekdays}, Action: action, Days: len(ends)})
		}

		for app, cameForward := range appFirst {
			if app == action.App {
				continue
			}
			n := 0
			for d, at := range cameForward {
				if then, ok := seen[d]; ok && !then.Before(at) && then.Sub(at) <= afterTheApp {
					n++
				}
			}
			if n >= daysToCount && float64(n)/float64(len(cameForward)) >= mostOften {
				consider(Timed{Trigger: Trigger{Kind: AfterAnApp, App: app}, Action: action, Days: n})
			}
		}
	}

	var out []Timed
	for _, t := range best {
		t.Key = t.key()
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Days != out[j].Days {
			return out[i].Days > out[j].Days
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > mostTimed {
		out = out[:mostTimed]
	}
	return out
}

func lastActivity(day []store.Click, closed time.Time) bool {
	for _, e := range day {
		if e.At.After(closed) && e.Role != store.RoleMoment {
			return false
		}
	}
	return true
}

func atATime(seen map[string]time.Time) (Timed, bool) {
	type mark struct {
		day    string
		minute int
	}
	var marks []mark
	for d, at := range seen {
		marks = append(marks, mark{d, minuteOf(at)})
	}
	sort.Slice(marks, func(i, j int) bool { return marks[i].minute < marks[j].minute })

	from, to := 0, -1
	for i := range marks {
		j := i
		for j+1 < len(marks) && marks[j+1].minute-marks[i].minute <= 2*nearTheTime {
			j++
		}
		if j-i > to-from {
			from, to = i, j
		}
	}
	if to-from+1 < daysToCount {
		return Timed{}, false
	}
	cluster := marks[from : to+1]
	weekdays := true
	for _, m := range cluster {
		if !isWeekday(m.day) {
			weekdays = false
		}
	}
	return Timed{
		Trigger: Trigger{Kind: AtATime, Minute: cluster[len(cluster)/2].minute, Weekdays: weekdays},
		Days:    len(cluster),
	}, true
}

func (t Timed) Doing() string {
	if t.Action.Role == store.RoleFront {
		return "Open " + t.Action.App
	}
	return t.Action.Title + " in " + t.Action.App
}

func (t Timed) When() string {
	switch t.Trigger.Kind {
	case AtATime:
		at := time.Date(2000, 1, 1, t.Trigger.Minute/60, t.Trigger.Minute%60, 0, 0, time.Local).Format("15:04")
		if t.Trigger.Weekdays {
			return "around " + at + " on weekdays"
		}
		return "around " + at
	case BeforeDone:
		at := time.Date(2000, 1, 1, t.Trigger.Minute/60, t.Trigger.Minute%60, 0, 0, time.Local).Format("15:04")
		if t.Trigger.Weekdays {
			return "before you finish for the day, around " + at + " on weekdays"
		}
		return "before you finish for the day, around " + at
	case WhenOpened:
		return "after you open your Mac"
	case AfterAnApp:
		return "after you open " + t.Trigger.App
	}
	return ""
}

func (t Timed) Instruction() string {
	if t.Action.Role == store.RoleFront {
		return "Open " + t.Action.App
	}
	return instructionFor([]store.Click{t.Action})
}

func (t Timed) Line() string {
	doing := t.Doing()
	if t.Trigger.Kind == BeforeDone {
		return "Before you finish, " + strings.ToLower(doing[:1]) + doing[1:] + ", like usual?"
	}
	return strings.ToUpper(doing[:1]) + doing[1:] + ", like usual?"
}
