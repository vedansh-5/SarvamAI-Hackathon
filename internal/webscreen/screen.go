package webscreen

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/uiact"
)

//go:embed page.js
var readPage string

//go:embed act.js
var actOnPage string

type Page interface {
	AskFront(ctx context.Context, script string, out any) error
	ReadFront(ctx context.Context, script string, out any) (loaded bool, err error)
	OpenInFront(ctx context.Context, pageURL string) error
}

type ProfilePage interface {
	OpenPrivate(ctx context.Context, pageURL string) error
	OpenInOwnProfile(ctx context.Context, pageURL string) error
}

const (
	ProfileNew     = "new"
	ProfilePrivate = "private"
)

var ProfileWait = 1200 * time.Millisecond

func (s *Screen) OpenIn(ctx context.Context, pageURL, profile string) error {
	if profile == "" {
		return s.Open(ctx, pageURL)
	}
	p, ok := s.Page.(ProfilePage)
	if !ok {
		return fmt.Errorf("webscreen: this browser cannot open another profile")
	}
	var err error
	if profile == ProfilePrivate {
		err = p.OpenPrivate(ctx, pageURL)
	} else {
		err = p.OpenInOwnProfile(ctx, pageURL)
	}
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(ProfileWait):
	}
	_, err = s.read(ctx)
	return err
}

type Screen struct {
	Page    Page
	Browser string

	Raise func(ctx context.Context, app string) error

	RealClick func(x, y int)
	RealType  func(text string)
	Log       func(format string, args ...any)
}

var (
	LoadWait   = 12 * time.Second
	LoadPoll   = 120 * time.Millisecond
	AnswerWait = 1 * time.Second
	AnswerPoll = 90 * time.Millisecond
	HandPause  = 180 * time.Millisecond
)

const WebArea = "AXWebArea"

type control struct {
	Role    string `json:"role"`
	Title   string `json:"title"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
	Focused bool   `json:"focused"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	W       int    `json:"w"`
	H       int    `json:"h"`
}

type snapshot struct {
	URL   string `json:"url"`
	Host  string `json:"host"`
	Title string `json:"title"`
	Ready string `json:"ready"`
	View  struct {
		X int `json:"x"`
		Y int `json:"y"`
		W int `json:"w"`
		H int `json:"h"`
	} `json:"view"`
	Controls []control `json:"controls"`
}

func (s *Screen) browser() string {
	if s.Browser != "" {
		return s.Browser
	}
	return "Google Chrome"
}

func (s *Screen) read(ctx context.Context) (snapshot, error) {
	deadline := time.Now().Add(LoadWait)
	for {
		var snap snapshot
		loaded, err := s.Page.ReadFront(ctx, readPage, &snap)
		if err != nil {
			return snapshot{}, err
		}
		if loaded {
			return snap, nil
		}
		if time.Now().After(deadline) {
			if err := s.Page.AskFront(ctx, readPage, &snap); err != nil {
				return snapshot{}, err
			}
			return snap, nil
		}
		select {
		case <-ctx.Done():
			return snapshot{}, ctx.Err()
		case <-time.After(LoadPoll):
		}
	}
}

func (s *Screen) Capture(ctx context.Context, _ string) (uiact.Tree, error) {
	snap, err := s.read(ctx)
	if err != nil {
		return uiact.Tree{}, err
	}
	return treeOf(snap), nil
}

func (s *Screen) waitForAnAnswer(ctx context.Context, before uiact.Tree) bool {
	deadline := time.Now().Add(AnswerWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return true
		case <-time.After(AnswerPoll):
		}
		var snap snapshot
		loaded, err := s.Page.ReadFront(ctx, readPage, &snap)
		if err != nil || !loaded {
			return true
		}
		if len(uiact.Changes(before, treeOf(snap))) > 0 {
			return true
		}
	}
	return false
}

func (s *Screen) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}

func (s *Screen) byHand(ctx context.Context, step uiact.Step, before uiact.Tree) bool {
	verb := strings.ToLower(strings.TrimSpace(step.Do))
	if s.RealClick == nil || (verb != "click" && verb != "type") {
		return false
	}
	if verb == "type" && s.RealType == nil {
		return false
	}

	now, err := s.Capture(ctx, "")
	if err != nil {
		return false
	}
	x, y, w, h, why := now.RectFor(step.Target)
	if why != "" {
		return false
	}
	if s.Raise != nil {
		if err := s.Raise(ctx, s.browser()); err != nil {
			return false
		}
	}

	s.logf("the page did not answer a scripted %s, so it is done with the real pointer", verb)
	s.RealClick(x+w/2, y+h/2)
	if verb == "type" {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(HandPause):
		}
		s.RealType(step.Text)
	}
	return s.waitForAnAnswer(ctx, before)
}

func treeOf(snap snapshot) uiact.Tree {
	app := snap.Host
	if app == "" {
		app = snap.URL
	}
	tree := uiact.Tree{App: app, Window: snap.Title}
	tree.Elements = append(tree.Elements, uiact.Element{
		Role: WebArea, Enabled: true,
		Position: []int{snap.View.X, snap.View.Y},
		Size:     []int{snap.View.W, snap.View.H},
		Path:     []int{},
	})
	for i, c := range snap.Controls {
		tree.Elements = append(tree.Elements, uiact.Element{
			Role: c.Role, Title: c.Title, Value: c.Value,
			Enabled: c.Enabled, Focused: c.Focused,
			Position: []int{c.X, c.Y}, Size: []int{c.W, c.H},
			Path: []int{i},
		})
	}
	return tree
}

type act struct {
	Verb    string `json:"verb"`
	ID      int    `json:"id"`
	Text    string `json:"text,omitempty"`
	Key     string `json:"key,omitempty"`
	Code    string `json:"code,omitempty"`
	KeyCode int    `json:"keyCode,omitempty"`
	Meta    bool   `json:"meta,omitempty"`
	Ctrl    bool   `json:"ctrl,omitempty"`
	Alt     bool   `json:"alt,omitempty"`
	Shift   bool   `json:"shift,omitempty"`
}

var domKeys = map[string]act{
	"Enter":  {Key: "Enter", Code: "Enter", KeyCode: 13},
	"Escape": {Key: "Escape", Code: "Escape", KeyCode: 27},
	"Tab":    {Key: "Tab", Code: "Tab", KeyCode: 9},
	"Space":  {Key: " ", Code: "Space", KeyCode: 32},
	"Delete": {Key: "Backspace", Code: "Backspace", KeyCode: 8},
}

func (s *Screen) send(ctx context.Context, a act) error {
	arg, err := json.Marshal(a)
	if err != nil {
		return err
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := s.Page.AskFront(ctx, actOnPage+"("+string(arg)+")", &out); err != nil {
		return err
	}
	if !out.OK {
		return &uiact.InvalidError{Reasons: []string{out.Error}}
	}
	return nil
}

func idOf(el uiact.Element) (int, bool) {
	if len(el.Path) != 1 {
		return 0, false
	}
	return el.Path[0], true
}

func (s *Screen) Act(ctx context.Context, p uiact.Program, t uiact.Tree) error {
	if err := uiact.Validate(p, t); err != nil {
		return err
	}

	for _, step := range p.Steps {
		a := act{Verb: strings.ToLower(strings.TrimSpace(step.Do)), Text: step.Text}

		if a.Verb == "press" {
			key, ok := domKeys[step.Key]
			if !ok {
				return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("%q is not a key VANS presses on a page", step.Key)}}
			}
			a.Key, a.Code, a.KeyCode = key.Key, key.Code, key.KeyCode
			for _, m := range step.Modifiers {
				switch m {
				case "Command":
					a.Meta = true
				case "Control":
					a.Ctrl = true
				case "Option":
					a.Alt = true
				case "Shift":
					a.Shift = true
				}
			}
		} else {
			el, why := t.Resolve(step.Target)
			if why != "" {
				return &uiact.InvalidError{App: t.App, Reasons: []string{why}}
			}
			id, ok := idOf(el)
			if !ok {
				return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("the %s is not something on the page to act on", el.Role)}}
			}
			a.ID = id
		}

		if err := s.send(ctx, a); err != nil {
			var invalid *uiact.InvalidError
			if errors.As(err, &invalid) {
				invalid.App = t.App
			}
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !s.waitForAnAnswer(ctx, t) {
			s.byHand(ctx, step, t)
		}
	}
	return nil
}

func (s *Screen) ScrollIntoView(ctx context.Context, t uiact.Tree, sel uiact.Selector) error {
	el, why := t.Resolve(sel)
	if why != "" {
		return &uiact.InvalidError{App: t.App, Reasons: []string{why}}
	}
	id, ok := idOf(el)
	if !ok {
		return fmt.Errorf("webscreen: nothing to scroll to")
	}
	if err := s.send(ctx, act{Verb: "scroll", ID: id}); err != nil {
		return err
	}
	s.waitForAnAnswer(ctx, t)
	return ctx.Err()
}

func (s *Screen) Open(ctx context.Context, pageURL string) error {
	if err := s.Page.OpenInFront(ctx, pageURL); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * LoadPoll):
	}
	_, err := s.read(ctx)
	return err
}

func (s *Screen) Frontmost(context.Context) (string, error) { return s.browser(), nil }

func (s *Screen) BringToFront(ctx context.Context, _ string) error {
	if s.Raise == nil {
		return nil
	}
	return s.Raise(ctx, s.browser())
}

func (s *Screen) Type(context.Context, string) error {
	return fmt.Errorf("webscreen: typing at the cursor is not done through the page")
}
