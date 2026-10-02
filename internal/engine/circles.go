package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/uiact"
)

var (
	lateFor     = 1200 * time.Millisecond
	arrivalPoll = 150 * time.Millisecond
)

const (
	mostStillTries  = 2
	mostSameTyping  = 2
	mostSameClicks  = 8
	backAndForthMin = 3
	mostSamePair    = 2
	mostWarnings    = 2
)

func (e *Engine) waitForItToArrive(ctx context.Context, app string, step uiact.Step) (uiact.Tree, bool) {
	var latest uiact.Tree
	deadline := time.Now().Add(lateFor)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return latest, false
		case <-time.After(arrivalPoll):
		}
		fresh, err := e.UIScreen.Capture(ctx, app)
		if err != nil || len(fresh.Elements) == 0 {
			continue
		}
		latest = fresh
		if uiact.Validate(uiact.Program{App: fresh.App, Steps: []uiact.Step{step}}, fresh) == nil {
			return fresh, true
		}
	}
	return latest, false
}

func stepKey(s uiact.Step) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(s.Do)),
		s.Target.Role,
		strings.ToLower(strings.TrimSpace(s.Target.Title)),
		fmt.Sprint(s.Target.Index),
		s.Text,
		s.Key,
		strings.Join(s.Modifiers, "+"),
	}, "\x1f")
}

type stepsSoFar struct {
	keys   []string
	names  []string
	still  map[string]int
	warned int
}

func (h *stepsSoFar) pairs(from, to string) int {
	n := 0
	for i := 1; i < len(h.keys); i++ {
		if h.keys[i-1] == from && h.keys[i] == to {
			n++
		}
	}
	return n
}

func (h *stepsSoFar) did(step uiact.Step, describe string) {
	h.keys = append(h.keys, stepKey(step))
	h.names = append(h.names, describe)
}

func (h *stepsSoFar) changedNothing(step uiact.Step) {
	if h.still == nil {
		h.still = map[string]int{}
	}
	h.still[stepKey(step)]++
}

func (h *stepsSoFar) goingInCircles(step uiact.Step, describe string) string {
	key := stepKey(step)

	if h.still[key] >= mostStillTries {
		return fmt.Sprintf("%q has been tried %d times and changed nothing on screen, so it is not the way forward; do something else, or say stuck",
			describe, h.still[key])
	}

	same := 0
	for _, k := range h.keys {
		if k == key {
			same++
		}
	}
	most := mostSameClicks
	if strings.EqualFold(strings.TrimSpace(step.Do), "type") {
		most = mostSameTyping
	}
	if same >= most {
		return fmt.Sprintf("%q has already been done %d times; doing it again will not help, so do something else, or say stuck",
			describe, same)
	}

	n := len(h.keys)
	if n >= backAndForthMin && h.keys[n-2] == key && h.keys[n-1] == h.keys[n-3] && h.keys[n-1] != key {
		return fmt.Sprintf("the last steps went back and forth between %q and %q without getting anywhere; do something else, or say stuck",
			h.names[n-1], describe)
	}
	if n >= 1 && h.keys[n-1] != key && h.pairs(h.keys[n-1], key) >= mostSamePair {
		return fmt.Sprintf("%q followed by %q has already happened %d times and the task is no further on; do something else, or say stuck",
			h.names[n-1], describe, mostSamePair)
	}
	return ""
}
