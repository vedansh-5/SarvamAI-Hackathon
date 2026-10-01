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
	FrontLoading(ctx context.Context) (bool, error)
	OpenInFront(ctx context.Context, pageURL string) error
}

type Screen struct {
	Page    Page
	Browser string

	Raise   func(ctx context.Context, app string) error
	Pointer func() (int, int, bool)
}

var (
	LoadWait    = 12 * time.Second
	LoadPoll    = 150 * time.Millisecond
	AfterActing = 350 * time.Millisecond
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

func (s *Screen) waitForTheLoad(ctx context.Context) error {
	deadline := time.Now().Add(LoadWait)
	for {
		loading, err := s.Page.FrontLoading(ctx)
		if err != nil {
			return err
		}
		if !loading || time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(LoadPoll):
		}
	}
}

func (s *Screen) Capture(ctx context.Context, _ string) (uiact.Tree, error) {
	if err := s.waitForTheLoad(ctx); err != nil {
		return uiact.Tree{}, err
	}

	var snap snapshot
	if err := s.Page.AskFront(ctx, readPage, &snap); err != nil {
		return uiact.Tree{}, err
	}
	return treeOf(snap), nil
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

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(AfterActing):
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
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(AfterActing):
		return nil
	}
}

func (s *Screen) Open(ctx context.Context, pageURL string) error {
	if err := s.Page.OpenInFront(ctx, pageURL); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(AfterActing):
	}
	return s.waitForTheLoad(ctx)
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

func (s *Screen) PointerAt() (int, int, bool) {
	if s.Pointer == nil {
		return 0, 0, false
	}
	return s.Pointer()
}
