package rpc

import (
	"context"
	"encoding/json"

	"github.com/vedansh/vans/internal/store"
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
	return map[string]bool{"kept": kept}, err
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
	return s.watching(ctx)
}
