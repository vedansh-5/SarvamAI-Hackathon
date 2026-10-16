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
	Shared bool `json:"shared"`
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

func (s *Server) moment(ctx context.Context, req Request) (map[string]bool, error) {
	var p struct {
		Kind string `json:"kind"`
		App  string `json:"app"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &Error{Code: CodeBadRequest, Message: err.Error()}
	}
	if s.busyOnScreen() {
		return map[string]bool{"kept": false}, nil
	}
	kept, err := s.Engine.NoteMoment(ctx, p.Kind, p.App)
	if err != nil || !kept {
		return map[string]bool{"kept": kept}, err
	}
	if s.timeToLook() {
		s.lookForHabits(context.WithoutCancel(ctx))
	}
	switch p.Kind {
	case store.RoleFront:
		s.offer(ctx, engine.Cause{Kind: engine.AppForward, App: p.App})
	case "unlock", "wake":
		s.offer(ctx, engine.Cause{Kind: engine.MacOpened})
	}
	return map[string]bool{"kept": kept}, nil
}

const offerTick = time.Minute

func (s *Server) offerByTheClock(ctx context.Context) {
	if s.Engine == nil || s.Engine.Clicks == nil {
		return
	}
	tick := time.NewTicker(offerTick)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.offer(ctx, engine.Cause{Kind: engine.ByTheClock})
		}
	}
}

const holdOfflineFor = 2 * time.Hour

type heldOffer struct {
	offer engine.Offer
	since time.Time
}

func (s *Server) online(ctx context.Context) bool {
	return s.Online == nil || s.Online(ctx)
}

func (s *Server) offer(ctx context.Context, cause engine.Cause) {
	if s.busyOnScreen() {
		return
	}
	now := time.Now()
	offers, err := s.Engine.OffersFor(ctx, cause, now)
	if err != nil {
		s.notify(0, "could not work out what to offer: %v", err)
		return
	}
	s.mu.Lock()
	waiting := s.stillHeld(now)
	s.mu.Unlock()
	for _, o := range offers {
		waiting = append(waiting, heldOffer{offer: o, since: now})
	}
	needs := false
	for _, w := range waiting {
		needs = needs || w.offer.NeedsInternet
	}
	up := !needs || s.online(ctx)
	for _, w := range waiting {
		if w.offer.NeedsInternet && !up {
			s.mu.Lock()
			s.held = append(s.held, w)
			s.mu.Unlock()
			if w.since.Equal(now) {
				s.notify(0, "holding until the internet is back: %s", w.offer.Line)
			}
			continue
		}
		s.notify(0, "offering: %s", w.offer)
		s.push(0, "offer", w.offer)
	}
}

func (s *Server) stillHeld(now time.Time) []heldOffer {
	var out []heldOffer
	for _, h := range s.held {
		if now.Sub(h.since) < holdOfflineFor {
			out = append(out, h)
		}
	}
	s.held = nil
	return out
}

func (s *Server) answerOffer(ctx context.Context, req Request) (engine.OfferAnswer, error) {
	var p struct {
		Key    string `json:"key"`
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return engine.OfferAnswer{}, &Error{Code: CodeBadRequest, Message: err.Error()}
	}
	got, err := s.Engine.AnswerOffer(ctx, p.Key, p.Answer, time.Now())
	if err != nil {
		return engine.OfferAnswer{}, &Error{Code: CodeBadRequest, Message: err.Error()}
	}
	go s.share(context.WithoutCancel(ctx))
	return got, nil
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
	go s.share(ctx)
}

func (s *Server) share(ctx context.Context) {
	if err := s.Engine.ShareWhatWasLearned(ctx); err != nil {
		s.notify(0, "could not send what was learned: %v", err)
	}
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
	go s.share(context.WithoutCancel(ctx))
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
	return WatchingData{On: on, Clicks: len(clicks), Shared: s.Engine.Learner != nil}, err
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
