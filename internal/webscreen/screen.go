package webscreen

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
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
	RealKey   func(code int, flags uint64)
	RealMove  func(x, y int)
	RealDrag  func(x1, y1, x2, y2 int)
	Choose    func(ctx context.Context, app, path string) error
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
	Role    string  `json:"role"`
	Title   string  `json:"title"`
	Value   string  `json:"value"`
	Enabled bool    `json:"enabled"`
	Focused bool    `json:"focused"`
	Notice  bool    `json:"notice"`
	Low     float64 `json:"low"`
	High    float64 `json:"high"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	W       int     `json:"w"`
	H       int     `json:"h"`
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
			Ranged: c.Role == "AXSlider" && c.High > c.Low, Low: c.Low, High: c.High,
			Enabled: c.Enabled, Focused: c.Focused, Notice: c.Notice,
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
	"Enter":    {Key: "Enter", Code: "Enter", KeyCode: 13},
	"Escape":   {Key: "Escape", Code: "Escape", KeyCode: 27},
	"Tab":      {Key: "Tab", Code: "Tab", KeyCode: 9},
	"Space":    {Key: " ", Code: "Space", KeyCode: 32},
	"Delete":   {Key: "Backspace", Code: "Backspace", KeyCode: 8},
	"Up":       {Key: "ArrowUp", Code: "ArrowUp", KeyCode: 38},
	"Down":     {Key: "ArrowDown", Code: "ArrowDown", KeyCode: 40},
	"Left":     {Key: "ArrowLeft", Code: "ArrowLeft", KeyCode: 37},
	"Right":    {Key: "ArrowRight", Code: "ArrowRight", KeyCode: 39},
	"PageUp":   {Key: "PageUp", Code: "PageUp", KeyCode: 33},
	"PageDown": {Key: "PageDown", Code: "PageDown", KeyCode: 34},
	"Home":     {Key: "Home", Code: "Home", KeyCode: 36},
	"End":      {Key: "End", Code: "End", KeyCode: 35},
}

func (s *Screen) pressForReal(ctx context.Context, step uiact.Step) error {
	if s.RealKey == nil {
		return fmt.Errorf("webscreen: this build cannot press %s on a page", step.Chord())
	}
	if s.Raise != nil {
		if err := s.Raise(ctx, s.browser()); err != nil {
			return err
		}
	}
	s.RealKey(uiact.Keys[step.Key], step.HeldDown())
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(HandPause):
	}
	return nil
}

func (s *Screen) chooseFile(ctx context.Context, step uiact.Step, t uiact.Tree) error {
	if s.RealClick == nil || s.Choose == nil {
		return fmt.Errorf("webscreen: this build cannot choose a file on a page")
	}
	x, y, w, h, why := t.RectFor(step.Target)
	if why != "" {
		return &uiact.InvalidError{App: t.App, Reasons: []string{why}}
	}
	if s.Raise != nil {
		if err := s.Raise(ctx, s.browser()); err != nil {
			return err
		}
	}
	s.logf("opening the file chooser with the real pointer, which is the only click a page accepts for it")
	s.RealClick(x+w/2, y+h/2)
	return s.Choose(ctx, s.browser(), step.Text)
}

func (s *Screen) send(ctx context.Context, a act) error {
	out, err := s.ask(ctx, a)
	if err != nil {
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

		if step.Switches() || step.Waits() {
			continue
		}
		if page, ok := pageVerbs[a.Verb]; ok {
			a.Verb = page
		}
		if step.Verb() == "drag" {
			if err := s.dragForReal(ctx, step, t); err != nil {
				return err
			}
			s.waitForAnAnswer(ctx, t)
			continue
		}
		if step.Chooses() {
			if err := s.chooseFile(ctx, step, t); err != nil {
				return err
			}
			s.waitForAnAnswer(ctx, t)
			continue
		}
		if a.Verb == "press" && uiact.NeedsCommand[step.Key] {
			if err := s.pressForReal(ctx, step); err != nil {
				return err
			}
			s.waitForAnAnswer(ctx, t)
			continue
		}

		var el uiact.Element
		if a.Verb == "press" {
			key, ok := domKeys[step.Key]
			if !ok {
				return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("%q is not a key Vspk presses on a page", step.Key)}}
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
			var why string
			el, why = t.Resolve(step.Target)
			if why != "" {
				return &uiact.InvalidError{App: t.App, Reasons: []string{why}}
			}
			id, ok := idOf(el)
			if !ok && step.Verb() == "scroll" {
				id, ok = -1, true
			}
			if !ok {
				return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("the %s is not something on the page to act on", el.Role)}}
			}
			a.ID = id
		}

		var err error
		switch {
		case step.Verb() == "type" && el.Role == "AXDateField":
			err = s.pickDate(ctx, t, step)
		case step.Verb() == "set":
			err = s.setCount(ctx, step)
		default:
			err = s.actOn(ctx, a, step, el, t)
		}
		if err != nil {
			var invalid *uiact.InvalidError
			if errors.As(err, &invalid) {
				invalid.App = t.App
			}
			return err
		}
	}
	return nil
}

func (s *Screen) actOn(ctx context.Context, a act, step uiact.Step, el uiact.Element, t uiact.Tree) error {
	out, err := s.ask(ctx, a)
	if err != nil {
		return err
	}
	if out.Opens && step.Verb() == "type" {
		return s.typeWhereItOpens(ctx, t, a.ID, el, step.Text)
	}
	if !out.OK {
		return &uiact.InvalidError{App: t.App, Reasons: []string{out.Error}}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !s.waitForAnAnswer(ctx, t) {
		if step.Verb() == "hover" {
			s.pointForReal(ctx, step, t)
			return nil
		}
		s.byHand(ctx, step, t)
	}
	if step.Verb() == "type" && el.Role == "AXComboBox" {
		s.pickSuggestion(ctx, t, step.Text)
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

func (s *Screen) Selection(ctx context.Context, _ string) (string, bool) {
	var picked string
	if err := s.Page.AskFront(ctx, "JSON.stringify(String(window.getSelection()))", &picked); err != nil {
		return "", false
	}
	picked = strings.TrimSpace(picked)
	return picked, picked != ""
}

func (s *Screen) ValueOf(ctx context.Context, t uiact.Tree, sel uiact.Selector) (string, bool) {
	el, why := t.Resolve(sel)
	if why != "" || el.Role == "AXSecureTextField" {
		return "", false
	}
	id, ok := idOf(el)
	if !ok {
		return "", false
	}
	if el.Role == "AXDateField" {
		if at, ok := wantedDate(strings.TrimSpace(el.Value+" "+el.Title), time.Now()); ok {
			return at.Format("2 Jan 2006"), true
		}
	}
	if el.Role == "AXIncrementor" {
		if _, err := strconv.Atoi(strings.TrimSpace(el.Value)); err == nil {
			return strings.TrimSpace(el.Value), true
		}
	}
	var out struct {
		OK    bool   `json:"ok"`
		Value string `json:"value"`
	}
	script := fmt.Sprintf(`(function(){var e=(window.__vansEls||[])[%d];`+
		`if(!e||!e.isConnected)return JSON.stringify({ok:false});`+
		`var t=e.tagName.toLowerCase();`+
		`if(t!=='input'&&t!=='textarea')return JSON.stringify({ok:false});`+
		`if((e.getAttribute('type')||'').toLowerCase()==='password')return JSON.stringify({ok:false});`+
		`return JSON.stringify({ok:true,value:String(e.value)})})()`, id)
	if err := s.Page.AskFront(ctx, script, &out); err != nil || !out.OK {
		return "", false
	}
	return out.Value, true
}

func (s *Screen) PutBack(ctx context.Context, _ string, sel uiact.Selector, value string) error {
	tree, err := s.Capture(ctx, "")
	if err != nil {
		return err
	}
	el, why := tree.Resolve(sel)
	if why != "" {
		return fmt.Errorf("webscreen: %s, so there is nowhere to put the text back", why)
	}
	id, ok := idOf(el)
	if !ok {
		return fmt.Errorf("webscreen: that is not a field on the page")
	}
	if el.Role == "AXIncrementor" {
		return s.Act(ctx, uiact.Program{App: tree.App, Steps: []uiact.Step{{Do: "set", Target: sel, Text: value}}}, tree)
	}
	if value != "" {
		return s.Act(ctx, uiact.Program{App: tree.App, Steps: []uiact.Step{{Do: "type", Target: sel, Text: value}}}, tree)
	}
	return s.send(ctx, act{Verb: "type", ID: id, Text: value})
}

var pageVerbs = map[string]string{
	"doubleclick": "dblclick",
	"rightclick":  "contextmenu",
	"scroll":      "scrollby",
}

func (s *Screen) dragForReal(ctx context.Context, step uiact.Step, t uiact.Tree) error {
	if s.RealDrag == nil || step.To == nil {
		return fmt.Errorf("webscreen: this build cannot drag on a page")
	}
	x, y, w, h, why := t.RectFor(step.Target)
	if why != "" {
		return &uiact.InvalidError{App: t.App, Reasons: []string{why}}
	}
	tx, ty, tw, th, why := t.RectFor(*step.To)
	if why != "" {
		return &uiact.InvalidError{App: t.App, Reasons: []string{why}}
	}
	if s.Raise != nil {
		if err := s.Raise(ctx, s.browser()); err != nil {
			return err
		}
	}
	s.logf("dragging with the real pointer, which is the only drag a page answers")
	s.RealDrag(x+w/2, y+h/2, tx+tw/2, ty+th/2)
	return nil
}

func (s *Screen) pointForReal(ctx context.Context, step uiact.Step, t uiact.Tree) {
	if s.RealMove == nil {
		return
	}
	x, y, w, h, why := t.RectFor(step.Target)
	if why != "" {
		return
	}
	if s.Raise != nil && s.Raise(ctx, s.browser()) != nil {
		return
	}
	s.logf("the page did not answer a scripted hover, so the real pointer is rested there")
	s.RealMove(x+w/2, y+h/2)
	s.waitForAnAnswer(ctx, t)
}

const mostPageText = 20000

func (s *Screen) ReadText(ctx context.Context, _ string) (uiact.ScreenText, error) {
	var out struct {
		Host  string `json:"host"`
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	script := fmt.Sprintf(`JSON.stringify({host: location.hostname.replace(/^www\./, ''), title: document.title, `+
		`text: String(document.body ? document.body.innerText : '').slice(0, %d)})`, mostPageText)
	if err := s.Page.AskFront(ctx, script, &out); err != nil {
		return uiact.ScreenText{}, err
	}
	var lines []string
	for _, line := range strings.Split(out.Text, "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return uiact.ScreenText{App: out.Host, Window: out.Title, Lines: lines}, nil
}
