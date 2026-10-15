package rpc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vedansh/vans/internal/engine"
	"github.com/vedansh/vans/internal/store"
)

const (
	lookEvery     = 20
	lookAtMostPer = 5 * time.Minute
)

type WatchingData struct {
	On     bool `json:"on"`
	Clicks int  `json:"clicks"`
}

func (s *Server) busyOnScreen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.guideCancel != nil || s.actCancel != nil
}

func (s *Server) noticed(ctx context.Context, req Request) (map[string]bool, error) {
	var p struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &Error{Code: CodeBadRequest, Message: err.Error()}
	}
	if s.busyOnScreen() {
		return map[string]bool{"kept": false}, nil
	}
	kept, err := s.Engine.NoteClick(ctx, p.X, p.Y)
	if kept && s.timeToLook() {
		go s.lookForHabits(context.WithoutCancel(ctx))
	}
	return map[string]bool{"kept": kept}, err
}

func (s *Server) timeToLook() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noted++
	if s.noted < lookEvery || time.Since(s.lookedAt) < lookAtMostPer {
		return false
	}
	s.noted, s.lookedAt = 0, time.Now()
	return true
}

func (s *Server) lookForHabits(ctx context.Context) {
	if _, err := s.Engine.LookForHabits(ctx); err != nil {
		s.notify(0, "could not look for habits: %v", err)
		return
	}
	s.tellHabits(ctx)
}

func (s *Server) tellHabits(ctx context.Context) {
	offers, err := s.Engine.HabitOffers(ctx)
	if err != nil {
		return
	}
	s.push(0, "habits", map[string]int{"open": len(offers)})
}

func (s *Server) habits(ctx context.Context) ([]engine.HabitOffer, error) {
	offers, err := s.Engine.HabitOffers(ctx)
	if offers == nil {
		offers = []engine.HabitOffer{}
	}
	return offers, err
}

func (s *Server) settleHabit(ctx context.Context, req Request) (map[string]string, error) {
	var p struct {
		Key  string `json:"key"`
		How  string `json:"how"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &Error{Code: CodeBadRequest, Message: err.Error()}
	}
	instruction, err := s.Engine.SettleHabit(ctx, p.Key, p.How, p.Name)
	if err != nil {
		return nil, &Error{Code: CodeBadRequest, Message: err.Error()}
	}
	s.tellHabits(ctx)
	return map[string]string{"instruction": instruction}, nil
}

func (s *Server) watching(ctx context.Context) (WatchingData, error) {
	if s.Engine.Clicks == nil {
		return WatchingData{}, nil
	}
	on, err := s.Engine.Clicks.Watching(ctx)
	if err != nil {
		return WatchingData{}, err
	}
	clicks, err := s.Engine.NotedClicks(ctx)
	return WatchingData{On: on, Clicks: len(clicks)}, err
}

func (s *Server) setWatching(ctx context.Context, req Request) (WatchingData, error) {
	var p struct {
		On bool `json:"on"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return WatchingData{}, &Error{Code: CodeBadRequest, Message: err.Error()}
	}
	if s.Engine.Clicks == nil {
		return WatchingData{}, &Error{Code: CodeBadRequest, Message: "this build does not watch"}
	}
	if err := s.Engine.Clicks.SetWatching(ctx, p.On); err != nil {
		return WatchingData{}, err
	}
	return s.watching(ctx)
}

func (s *Server) notedClicks(ctx context.Context) ([]store.Click, error) {
	clicks, err := s.Engine.NotedClicks(ctx)
	if clicks == nil {
		clicks = []store.Click{}
	}
	return clicks, err
}

func (s *Server) clearClicks(ctx context.Context) (WatchingData, error) {
	if s.Engine.Clicks != nil {
		if err := s.Engine.Clicks.ClearClicks(ctx); err != nil {
			return WatchingData{}, err
		}
	}
	s.tellHabits(ctx)
	return s.watching(ctx)
}
