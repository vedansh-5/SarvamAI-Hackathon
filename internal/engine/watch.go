package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type ClickLog interface {
	Watching(ctx context.Context) (bool, error)
	SetWatching(ctx context.Context, on bool) error
	NoteClick(ctx context.Context, c store.Click) error
	Clicks(ctx context.Context, since time.Time) ([]store.Click, error)
	ForgetClicksBefore(ctx context.Context, before time.Time) error
	ClearClicks(ctx context.Context) error
	KeepHabit(ctx context.Context, h store.Habit) (bool, error)
	OpenHabits(ctx context.Context) ([]store.Habit, error)
	Habit(ctx context.Context, key string) (store.Habit, bool, error)
	SettleHabit(ctx context.Context, key, state string) error
	KeepTimed(ctx context.Context, r store.TimedRow) error
	TimedRows(ctx context.Context) ([]store.TimedRow, error)
	TimedRow(ctx context.Context, key string) (store.TimedRow, bool, error)
	PutTimed(ctx context.Context, r store.TimedRow) error
	Setting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
	AllHabits(ctx context.Context) ([]store.HabitState, error)
	AllTimed(ctx context.Context) ([]store.TimedRow, error)
}

type LearnedSender interface {
	SendLearned(ctx context.Context, l synth.Learned) error
}

const sharedKey = "learned.shared"

func (e *Engine) WhatWasLearned(ctx context.Context) (synth.Learned, error) {
	var l synth.Learned
	habits, err := e.Clicks.AllHabits(ctx)
	if err != nil {
		return l, err
	}
	for _, h := range habits {
		l.Habits = append(l.Habits, synth.LearnedHabit{What: h.Name, Times: h.Times, Answer: h.State})
	}
	timed, err := e.Clicks.AllTimed(ctx)
	if err != nil {
		return l, err
	}
	for _, r := range timed {
		t, ok := timedFrom(r)
		if !ok {
			continue
		}
		l.Habits = append(l.Habits, synth.LearnedHabit{
			What: t.Doing(), When: t.When(), Days: r.Days,
			Yes: r.Yes, Skips: r.Skips, Always: r.Always, Stopped: r.Stopped,
		})
	}
	return l, nil
}

func (e *Engine) ShareWhatWasLearned(ctx context.Context) error {
	if e.Clicks == nil || e.Learner == nil {
		return nil
	}
	l, err := e.WhatWasLearned(ctx)
	if err != nil || len(l.Habits) == 0 {
		return err
	}
	raw, err := json.Marshal(l)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	stamp := hex.EncodeToString(sum[:])
	if was, _ := e.Clicks.Setting(ctx, sharedKey); was == stamp {
		return nil
	}
	if err := e.Learner.SendLearned(ctx, l); err != nil {
		return err
	}
	return e.Clicks.SetSetting(ctx, sharedKey, stamp)
}

const keepClicksFor = 7 * 24 * time.Hour

func (e *Engine) NoteClick(ctx context.Context, x, y int) (bool, error) {
	if e.Clicks == nil {
		return false, nil
	}
	on, err := e.Clicks.Watching(ctx)
	if err != nil || !on {
		return false, err
	}
	at := e.ClickAt
	if at == nil {
		at = uiact.NoticeAt
	}
	seen, ok := at(x, y)
	if !ok {
		return false, nil
	}
	now := time.Now()
	if err := e.Clicks.NoteClick(ctx, store.Click{At: now, App: seen.App, Role: seen.Role, Title: seen.Title}); err != nil {
		return false, err
	}
	if err := e.Clicks.ForgetClicksBefore(ctx, now.Add(-keepClicksFor)); err != nil {
		e.logf("could not drop clicks older than a week: %v", err)
	}
	return true, nil
}

var macMoments = map[string]bool{"wake": true, "sleep": true, "lock": true, "unlock": true}

func (e *Engine) NoteMoment(ctx context.Context, kind, app string) (bool, error) {
	if e.Clicks == nil {
		return false, nil
	}
	note := store.Click{At: time.Now()}
	switch {
	case kind == store.RoleFront && uiact.Watched(app):
		note.App, note.Role = strings.TrimSpace(app), store.RoleFront
	case macMoments[kind]:
		note.App, note.Role, note.Title = store.MacApp, store.RoleMoment, kind
	default:
		return false, nil
	}
	on, err := e.Clicks.Watching(ctx)
	if err != nil || !on {
		return false, err
	}
	if err := e.Clicks.NoteClick(ctx, note); err != nil {
		return false, err
	}
	return true, nil
}

func (e *Engine) NotedClicks(ctx context.Context) ([]store.Click, error) {
	if e.Clicks == nil {
		return nil, nil
	}
	return e.Clicks.Clicks(ctx, time.Now().Add(-keepClicksFor))
}

type HabitOffer struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Times       int    `json:"times"`
	Routine     string `json:"routine"`
	Instruction string `json:"instruction"`
}

func (e *Engine) LookForHabits(ctx context.Context) (int, error) {
	if e.Clicks == nil {
		return 0, nil
	}
	clicks, err := e.NotedClicks(ctx)
	if err != nil {
		return 0, err
	}
	for _, t := range FindTimed(clicks) {
		if err := e.keepTimed(ctx, t); err != nil {
			return 0, err
		}
	}
	open := 0
	for _, h := range FindHabits(clicks) {
		kept, err := e.Clicks.KeepHabit(ctx, store.Habit{Key: h.Key, Name: h.Name, Steps: h.Steps, Times: h.Times})
		if err != nil {
			return open, err
		}
		if kept {
			open++
		}
	}
	return open, nil
}

func routineNameFor(steps []store.Click) string {
	var words []string
	for i, c := range steps {
		if i == 3 {
			break
		}
		words = append(words, strings.ToLower(c.Title))
	}
	return strings.Join(words, " ")
}

func (e *Engine) HabitOffers(ctx context.Context) ([]HabitOffer, error) {
	if e.Clicks == nil {
		return nil, nil
	}
	habits, err := e.Clicks.OpenHabits(ctx)
	if err != nil {
		return nil, err
	}
	var out []HabitOffer
	for _, h := range habits {
		out = append(out, HabitOffer{
			Key: h.Key, Name: h.Name, Times: h.Times,
			Routine:     routineNameFor(h.Steps),
			Instruction: instructionFor(h.Steps),
		})
	}
	return out, nil
}

const (
	SaveHabit    = "save"
	ShowHabit    = "show"
	DismissHabit = "dismiss"
)

func (e *Engine) SettleHabit(ctx context.Context, key, how, name string) (string, error) {
	if e.Clicks == nil {
		return "", fmt.Errorf("engine: this build does not watch")
	}
	h, found, err := e.Clicks.Habit(ctx, key)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("engine: that habit has been cleared")
	}
	instruction := instructionFor(h.Steps)
	state := store.HabitDismissed
	switch how {
	case SaveHabit:
		if e.Routines == nil {
			return "", fmt.Errorf("engine: this build keeps no routines")
		}
		if strings.TrimSpace(name) == "" {
			name = routineNameFor(h.Steps)
		}
		if err := e.Routines.SaveRoutine(ctx, name, instruction); err != nil {
			return "", err
		}
		state = store.HabitSaved
	case ShowHabit:
		state = store.HabitShown
	case DismissHabit:
	default:
		return "", fmt.Errorf("engine: %q is not save, show or dismiss", how)
	}
	return instruction, e.Clicks.SettleHabit(ctx, key, state)
}
