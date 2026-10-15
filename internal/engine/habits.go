package engine

import (
	"sort"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/store"
)

type Habit struct {
	Key       string        `json:"key"`
	Name      string        `json:"name"`
	Steps     []store.Click `json:"steps"`
	Times     int           `json:"times"`
	Occasions int           `json:"occasions"`
}

const (
	doubleClick     = 1500 * time.Millisecond
	newOccasion     = 2 * time.Minute
	shortestHabit   = 3
	longestHabit    = 8
	timesToBeHabit  = 3
	occasionsNeeded = 2
	mostHabits      = 5
)

func clickKey(c store.Click) string {
	return c.App + "\x1f" + c.Role + "\x1f" + c.Title
}

func occasions(clicks []store.Click) [][]store.Click {
	var out [][]store.Click
	var now []store.Click
	for i, c := range clicks {
		if i > 0 {
			gap := c.At.Sub(clicks[i-1].At)
			if gap > newOccasion {
				out = append(out, now)
				now = nil
			} else if gap < doubleClick && clickKey(c) == clickKey(clicks[i-1]) {
				continue
			}
		}
		now = append(now, c)
	}
	if len(now) > 0 {
		out = append(out, now)
	}
	return out
}

func varied(run []store.Click) bool {
	seen := map[string]bool{}
	for _, c := range run {
		seen[clickKey(c)] = true
	}
	return len(seen) >= 2
}

func runKey(run []store.Click) string {
	keys := make([]string, len(run))
	for i, c := range run {
		keys[i] = clickKey(c)
	}
	return strings.Join(keys, "\x1e")
}

func FindHabits(clicks []store.Click) []Habit {
	type tally struct {
		habit Habit
		where map[int]bool
		ends  map[int]int
	}
	stretches := occasions(clicks)
	found := map[string]*tally{}
	for n := longestHabit; n >= shortestHabit; n-- {
		for s, stretch := range stretches {
			for i := 0; i+n <= len(stretch); i++ {
				run := stretch[i : i+n]
				if !varied(run) {
					continue
				}
				key := runKey(run)
				t := found[key]
				if t == nil {
					t = &tally{
						habit: Habit{Key: key, Steps: append([]store.Click{}, run...)},
						where: map[int]bool{}, ends: map[int]int{},
					}
					found[key] = t
				}
				if end, ok := t.ends[s]; ok && i < end {
					continue
				}
				t.ends[s] = i + n
				t.where[s] = true
				t.habit.Times++
			}
		}
	}

	var habits []Habit
	for _, t := range found {
		if t.habit.Times < timesToBeHabit || len(t.where) < occasionsNeeded {
			continue
		}
		t.habit.Occasions = len(t.where)
		t.habit.Name = habitName(t.habit.Steps)
		habits = append(habits, t.habit)
	}
	sort.Slice(habits, func(i, j int) bool {
		if len(habits[i].Steps) != len(habits[j].Steps) {
			return len(habits[i].Steps) > len(habits[j].Steps)
		}
		if habits[i].Times != habits[j].Times {
			return habits[i].Times > habits[j].Times
		}
		return habits[i].Key < habits[j].Key
	})

	var kept []Habit
	for _, h := range habits {
		inside := false
		for _, k := range kept {
			if h.Times <= k.Times && strings.Contains("\x1e"+k.Key+"\x1e", "\x1e"+h.Key+"\x1e") {
				inside = true
				break
			}
		}
		if !inside {
			kept = append(kept, h)
		}
		if len(kept) == mostHabits {
			break
		}
	}
	return kept
}

func habitName(steps []store.Click) string {
	var b strings.Builder
	app := ""
	for i, c := range steps {
		switch {
		case c.App != app && i == 0:
			b.WriteString("In " + c.App + ": ")
		case c.App != app:
			b.WriteString(", then in " + c.App + ": ")
		case i == len(steps)-1:
			b.WriteString(", then ")
		default:
			b.WriteString(", ")
		}
		b.WriteString(c.Title)
		app = c.App
	}
	return b.String()
}

func instructionFor(steps []store.Click) string {
	var parts []string
	app := ""
	for _, c := range steps {
		verb := "click " + c.Title
		if c.Role == "AXMenuItem" {
			verb = "choose " + c.Title
		}
		if c.App != app {
			verb = "in " + c.App + ", " + verb
			app = c.App
		}
		parts = append(parts, verb)
	}
	s := strings.Join(parts, ", then ")
	return strings.ToUpper(s[:1]) + s[1:]
}
