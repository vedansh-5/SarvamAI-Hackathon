package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/synth"
)

const (
	MostRequestBytes = 12 << 20
	HealthPath       = "/v1/health"
)

type Tester struct {
	Name  string
	Code  string
	Daily int
	Off   bool
}

type Ledger interface {
	Tester(ctx context.Context, code string) (Tester, bool, error)
	Spend(ctx context.Context, code, day string) (mine, all int, err error)
	Keep(ctx context.Context, f Feedback) error
	Record(ctx context.Context, a Activity) error
	KeepLearned(ctx context.Context, from string, at time.Time, habits string) error
}

type Feedback struct {
	ID      string
	From    string
	Kind    string
	Text    string
	At      time.Time
	Handled bool
}

type Activity struct {
	At    time.Time
	From  string
	Kind  string
	Words string
	App   string
	Fault string
	Took  time.Duration
	By    string
}

const MostWords = 500

var wordFields = []string{"Instruction", "Task", "Question", "Ask"}
var appFields = []string{"App", "InFront"}

func said(raw json.RawMessage) (words, app string) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "", ""
	}
	pick := func(names []string) string {
		for _, want := range names {
			for name, value := range fields {
				var text string
				if strings.EqualFold(name, want) && json.Unmarshal(value, &text) == nil && strings.TrimSpace(text) != "" {
					return strings.TrimSpace(text)
				}
			}
		}
		return ""
	}
	words = pick(wordFields)
	if runes := []rune(words); len(runes) > MostWords {
		words = string(runes[:MostWords]) + "…"
	}
	return words, pick(appFields)
}

func (s *Server) record(ctx context.Context, a Activity) {
	if err := s.Ledger.Record(ctx, a); err != nil {
		s.logf("could not record %s's %s: %v", a.From, a.Kind, err)
	}
}

type Mailer interface {
	Mail(ctx context.Context, subject, body string) error
}

type Server struct {
	Model    func(used *synth.Usage) synth.Model
	Ledger   Ledger
	Mail     Mailer
	Console  http.Handler
	Releases Releases
	Total    int
	Now      func() time.Time
	Log      func(format string, args ...any)
}

func (s *Server) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func Day(t time.Time) string { return t.UTC().Format("2006-01-02") }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == HealthPath && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	case s.Console != nil && (r.URL.Path == ConsolePath || strings.HasPrefix(r.URL.Path, ConsolePath+"/")):
		s.Console.ServeHTTP(w, r)
	case r.URL.Path == synth.AskPath && r.Method == http.MethodPost:
		s.ask(w, r)
	case strings.HasPrefix(r.URL.Path, UpdatePath) && r.Method == http.MethodGet:
		s.update(w, r)
	case strings.HasPrefix(r.URL.Path, UpdateFilePath) && r.Method == http.MethodGet:
		s.updateFile(w, r)
	case r.URL.Path == DownloadPath && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		s.download(w, r)
	case r.URL.Path == synth.AskPath:
		reply(w, http.StatusMethodNotAllowed, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultRefused, Message: "post only"}})
	default:
		reply(w, http.StatusNotFound, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultRefused, Message: "not found"}})
	}
}

func reply(w http.ResponseWriter, status int, out synth.Answered) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(out)
}

func refuse(w http.ResponseWriter, status int, message string) {
	reply(w, status, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultRefused, Message: message}})
}

func codeOf(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func (s *Server) tester(w http.ResponseWriter, r *http.Request) (Tester, bool) {
	code := codeOf(r)
	if code == "" {
		refuse(w, http.StatusUnauthorized, "this copy of VANS has no invite code")
		return Tester{}, false
	}
	who, found, err := s.Ledger.Tester(r.Context(), code)
	if err != nil {
		s.logf("could not look up an invite code: %v", err)
		reply(w, http.StatusServiceUnavailable, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultFailed, Message: "the VANS server could not check the invite code"}})
		return Tester{}, false
	}
	if !found {
		refuse(w, http.StatusUnauthorized, "this invite code is not one VANS knows")
		return Tester{}, false
	}
	if who.Off {
		refuse(w, http.StatusForbidden, "this invite code has been switched off")
		return Tester{}, false
	}
	return who, true
}

func (s *Server) ask(w http.ResponseWriter, r *http.Request) {
	who, ok := s.tester(w, r)
	if !ok {
		return
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, MostRequestBytes+1))
	if err != nil {
		refuse(w, http.StatusBadRequest, "the request could not be read")
		return
	}
	if len(data) > MostRequestBytes {
		refuse(w, http.StatusRequestEntityTooLarge, "the request is too large")
		return
	}
	var asked synth.Asking
	if err := json.Unmarshal(data, &asked); err != nil {
		refuse(w, http.StatusBadRequest, "the request is not readable")
		return
	}
	answerer, known := kinds[asked.Kind]
	if !known && asked.Kind != synth.KindFeedback && asked.Kind != synth.KindLearned {
		refuse(w, http.StatusBadRequest, fmt.Sprintf("%q is not a kind of question VANS asks", asked.Kind))
		return
	}

	if asked.Kind == synth.KindLearned {
		s.learned(w, r, who, asked.Request)
		return
	}

	words, app := said(asked.Request)
	seen := Activity{At: s.now(), From: who.Name, Kind: asked.Kind, Words: words, App: app}

	if asked.Kind != synth.KindPing {
		mine, all, err := s.Ledger.Spend(r.Context(), codeOf(r), Day(s.now()))
		if err != nil {
			s.logf("could not count a request for %s: %v", who.Name, err)
			reply(w, http.StatusServiceUnavailable, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultFailed, Message: "the VANS server could not count the request"}})
			return
		}
		if who.Daily > 0 && mine > who.Daily {
			s.logf("%s is over their %d for today", who.Name, who.Daily)
			seen.Fault = synth.FaultOverCap
			s.record(r.Context(), seen)
			reply(w, http.StatusTooManyRequests, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultOverCap,
				Message: "You have used today's allowance. It starts again at 5:30 in the morning, India time."}})
			return
		}
		if s.Total > 0 && all > s.Total {
			s.logf("every tester together is over %d for today", s.Total)
			seen.Fault = synth.FaultOverCap
			s.record(r.Context(), seen)
			reply(w, http.StatusTooManyRequests, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultOverCap,
				Message: "VANS has used today's allowance for everyone. It starts again at 5:30 in the morning, India time."}})
			return
		}
	}

	if asked.Kind == synth.KindFeedback {
		s.feedback(w, r, who, asked.Request)
		return
	}

	used := &synth.Usage{}
	started := time.Now()
	answered, err := answerer(r.Context(), s.Model(used), asked.Request)
	spent := used.Now()
	if err != nil {
		var bad *badRequest
		if errors.As(err, &bad) {
			refuse(w, http.StatusBadRequest, bad.Error())
			return
		}
		s.logf("%s asked %s: failed after %s and %d call(s): %v", who.Name, asked.Kind, time.Since(started).Round(10*time.Millisecond), spent.Calls, err)
		fault := synth.FaultOf(err)
		seen.Fault, seen.Took, seen.By = fault.Kind, time.Since(started), used.By()
		if asked.Kind != synth.KindPing {
			s.record(r.Context(), seen)
		}
		reply(w, http.StatusOK, synth.Answered{Fault: fault, Used: spent})
		return
	}
	encoded, err := json.Marshal(answered)
	if err != nil {
		s.logf("%s asked %s: the answer could not be encoded: %v", who.Name, asked.Kind, err)
		reply(w, http.StatusInternalServerError, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultFailed, Message: "the answer could not be sent"}, Used: spent})
		return
	}
	s.logf("%s asked %s: answered in %s with %d call(s)", who.Name, asked.Kind, time.Since(started).Round(10*time.Millisecond), spent.Calls)
	if asked.Kind != synth.KindPing {
		seen.Took, seen.By = time.Since(started), used.By()
		if choice, ok := answered.(synth.AppChoice); ok && choice.App != "" {
			seen.App = choice.App
		}
		s.record(r.Context(), seen)
	}
	reply(w, http.StatusOK, synth.Answered{Answer: encoded, Used: spent})
}

func (s *Server) feedback(w http.ResponseWriter, r *http.Request, who Tester, raw json.RawMessage) {
	var f synth.Feedback
	if err := json.Unmarshal(raw, &f); err != nil {
		refuse(w, http.StatusBadRequest, "the feedback is not readable")
		return
	}
	f.Text = strings.TrimSpace(f.Text)
	if f.Kind != synth.FeedbackFailedGuide && f.Kind != synth.FeedbackNote {
		refuse(w, http.StatusBadRequest, fmt.Sprintf("%q is not a kind of feedback VANS sends", f.Kind))
		return
	}
	if f.Text == "" || len(f.Text) > synth.MostFeedbackBytes {
		refuse(w, http.StatusBadRequest, "the feedback is empty or too long")
		return
	}
	kept := Feedback{From: who.Name, Kind: f.Kind, Text: f.Text, At: s.now()}
	if err := s.Ledger.Keep(r.Context(), kept); err != nil {
		s.logf("could not keep feedback from %s: %v", who.Name, err)
		reply(w, http.StatusServiceUnavailable, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultFailed, Message: "the VANS server could not keep the feedback"}})
		return
	}
	s.logf("%s sent %s feedback", who.Name, f.Kind)
	if s.Mail != nil {
		subject := fmt.Sprintf("VANS: %s from %s", strings.ReplaceAll(f.Kind, "_", " "), who.Name)
		if err := s.Mail.Mail(r.Context(), subject, f.Text); err != nil {
			s.logf("feedback from %s was kept but not mailed: %v", who.Name, err)
		}
	}
	reply(w, http.StatusOK, synth.Answered{})
}

func (s *Server) learned(w http.ResponseWriter, r *http.Request, who Tester, raw json.RawMessage) {
	var l synth.Learned
	if len(raw) > synth.MostLearnedBytes || json.Unmarshal(raw, &l) != nil {
		refuse(w, http.StatusBadRequest, "what was learned is not readable or too long")
		return
	}
	habits, err := json.Marshal(l.Habits)
	if err != nil {
		refuse(w, http.StatusBadRequest, "what was learned is not readable")
		return
	}
	if err := s.Ledger.KeepLearned(r.Context(), who.Name, s.now(), string(habits)); err != nil {
		s.logf("could not keep what was learned for %s: %v", who.Name, err)
		reply(w, http.StatusServiceUnavailable, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultFailed, Message: "the VANS server could not keep what was learned"}})
		return
	}
	s.logf("%s: %d learned habit(s) kept", who.Name, len(l.Habits))
	reply(w, http.StatusOK, synth.Answered{})
}

type badRequest struct{ kind string }

func (e *badRequest) Error() string { return fmt.Sprintf("the %s request is not readable", e.kind) }

type answerer func(ctx context.Context, m synth.Model, raw json.RawMessage) (any, error)

func answering[Q, A any](kind string, f func(m synth.Model, ctx context.Context, req Q) (A, error)) answerer {
	return func(ctx context.Context, m synth.Model, raw json.RawMessage) (any, error) {
		var req Q
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, &badRequest{kind: kind}
			}
		}
		return f(m, ctx, req)
	}
}

var kinds = map[string]answerer{
	synth.KindProgram:   answering(synth.KindProgram, synth.Model.Synthesize),
	synth.KindRoute:     answering(synth.KindRoute, synth.Model.Decide),
	synth.KindAnswer:    answering(synth.KindAnswer, synth.Model.Answer),
	synth.KindAppAction: answering(synth.KindAppAction, synth.Model.WriteAction),
	synth.KindWebAction: answering(synth.KindWebAction, synth.Model.WriteWebAction),
	synth.KindUIAction:  answering(synth.KindUIAction, synth.Model.WriteUIAction),
	synth.KindStep:      answering(synth.KindStep, synth.Model.WriteStep),
	synth.KindPlan:      answering(synth.KindPlan, synth.Model.WritePlan),
	synth.KindStyle:     answering(synth.KindStyle, synth.Model.WriteStyle),
	synth.KindWhichApp:  answering(synth.KindWhichApp, synth.Model.WhichApp),
	synth.KindText:      answering(synth.KindText, synth.Model.WorkOnText),
	synth.KindFiles:     answering(synth.KindFiles, synth.Model.WriteFileOps),
	synth.KindSee:       answering(synth.KindSee, synth.Model.SeeScreen),
	synth.KindRead:      answering(synth.KindRead, synth.Model.ReadScreen),
	synth.KindPing: func(ctx context.Context, m synth.Model, _ json.RawMessage) (any, error) {
		return struct{}{}, m.Ping(ctx)
	},
}
