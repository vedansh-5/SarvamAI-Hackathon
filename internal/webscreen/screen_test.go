package webscreen

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/uiact"
)

type fakePage struct {
	snap    snapshot
	loading int
	asked   []string
	refuse  string
	opened  []string
	askErr  error

	reads        int
	changeOnRead int
}

func (p *fakePage) AskFront(_ context.Context, script string, out any) error {
	if p.askErr != nil {
		return p.askErr
	}
	p.asked = append(p.asked, script)
	if strings.HasPrefix(script, actOnPage) {
		reply := `{"ok":true}`
		if p.refuse != "" {
			reply = `{"ok":false,"error":"` + p.refuse + `"}`
		}
		return json.Unmarshal([]byte(reply), out)
	}
	raw, _ := json.Marshal(p.snap)
	return json.Unmarshal(raw, out)
}

func (p *fakePage) ReadFront(ctx context.Context, script string, out any) (bool, error) {
	if p.loading > 0 {
		p.loading--
		return false, nil
	}
	p.reads++
	if p.changeOnRead > 0 && p.reads >= p.changeOnRead {
		p.snap.Title = "Results"
	}
	return true, p.AskFront(ctx, script, out)
}

func (p *fakePage) OpenInFront(_ context.Context, u string) error {
	p.opened = append(p.opened, u)
	return nil
}

func quick(t *testing.T) {
	t.Helper()
	wait, poll, answer, answerPoll := LoadWait, LoadPoll, AnswerWait, AnswerPoll
	LoadWait, LoadPoll, AnswerWait, AnswerPoll = 200*time.Millisecond, time.Millisecond, 30*time.Millisecond, time.Millisecond
	t.Cleanup(func() { LoadWait, LoadPoll, AnswerWait, AnswerPoll = wait, poll, answer, answerPoll })
}

func flights() snapshot {
	s := snapshot{URL: "https://www.example.com/flights", Host: "example.com", Title: "Flights"}
	s.View.X, s.View.Y, s.View.W, s.View.H = 0, 80, 1000, 700
	s.Controls = []control{
		{Role: "AXTextField", Title: "From", Enabled: true, X: 100, Y: 200, W: 200, H: 30},
		{Role: "AXButton", Title: "Search flights", Enabled: true, X: 100, Y: 300, W: 120, H: 30},
		{Role: "AXButton", Title: "Way down", Enabled: true, X: 100, Y: 2400, W: 120, H: 30},
	}
	return s
}

func TestAPageIsReadAsAWindowNamedForItsSite(t *testing.T) {
	quick(t)
	page := &fakePage{snap: flights(), loading: 3}
	s := &Screen{Page: page}

	tree, err := s.Capture(context.Background(), "Google Chrome")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if tree.App != "example.com" || tree.Window != "Flights" {
		t.Errorf("tree is %q / %q, want the site and the page title", tree.App, tree.Window)
	}
	if page.loading != 0 {
		t.Error("read the page while it was still loading")
	}
	if len(tree.Elements) != 4 || tree.Elements[0].Role != WebArea {
		t.Fatalf("elements = %+v, want the page area and three controls", tree.Elements)
	}

	x, y, w, h, why := tree.RectFor(uiact.Selector{Role: "AXButton", Title: "Search flights"})
	if why != "" || x != 100 || y != 300 || w != 120 || h != 30 {
		t.Errorf("rect = (%d,%d %dx%d) %q, want where the page put it", x, y, w, h, why)
	}
	if !tree.OutOfView(uiact.Selector{Role: "AXButton", Title: "Way down"}) {
		t.Error("a control below the bottom of the page area is not marked out of view")
	}
	if !strings.Contains(tree.Text(), `"Way down" [out of view]`) {
		t.Errorf("the model is not told it is out of view:\n%s", tree.Text())
	}
}

func TestActingSendsTheControlByItsPlaceInTheList(t *testing.T) {
	quick(t)
	page := &fakePage{snap: flights()}
	s := &Screen{Page: page}
	tree, _ := s.Capture(context.Background(), "")

	program := uiact.Program{App: tree.App, Steps: []uiact.Step{
		{Do: "type", Target: uiact.Selector{Role: "AXTextField", Title: "From"}, Text: "Delhi"},
		{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Search flights"}},
		{Do: "press", Key: "Enter", Target: uiact.Selector{Role: "AXTextField", Title: "From"}},
	}}
	if err := s.Act(context.Background(), program, tree); err != nil {
		t.Fatalf("Act: %v", err)
	}

	var sent []string
	for _, script := range page.asked {
		if strings.HasPrefix(script, actOnPage) {
			sent = append(sent, strings.TrimPrefix(script, actOnPage))
		}
	}
	if len(sent) != 3 {
		t.Fatalf("sent %d act(s), want 3: %v", len(sent), sent)
	}
	for i, want := range []string{`"verb":"type","id":0,"text":"Delhi"`, `"verb":"click","id":1`, `"verb":"press"`} {
		if !strings.Contains(sent[i], want) {
			t.Errorf("act %d = %s, want it to carry %s", i+1, sent[i], want)
		}
	}
	if !strings.Contains(sent[2], `"keyCode":13`) {
		t.Errorf("Enter was sent as %s", sent[2])
	}
}

func TestAControlThatLeftThePageIsARefusalNotACrash(t *testing.T) {
	quick(t)
	page := &fakePage{snap: flights(), refuse: "that control is no longer on the page"}
	s := &Screen{Page: page}
	tree, _ := s.Capture(context.Background(), "")

	err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: []uiact.Step{
		{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Search flights"}},
	}}, tree)

	var invalid *uiact.InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want a refusal the step loop can recover from", err)
	}
}

func TestAStepThatDoesNotFitThePageIsNeverSent(t *testing.T) {
	quick(t)
	page := &fakePage{snap: flights()}
	s := &Screen{Page: page}
	tree, _ := s.Capture(context.Background(), "")
	before := len(page.asked)

	err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: []uiact.Step{
		{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Pay"}},
	}}, tree)
	if err == nil {
		t.Fatal("acted on a control the page does not have")
	}
	if len(page.asked) != before {
		t.Error("something was sent to the page for a step that failed checking")
	}
}

func TestThePageAreaItselfCannotBeClicked(t *testing.T) {
	quick(t)
	page := &fakePage{snap: flights()}
	s := &Screen{Page: page}
	tree, _ := s.Capture(context.Background(), "")

	err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: []uiact.Step{
		{Do: "click", Target: uiact.Selector{Role: WebArea}},
	}}, tree)
	if err == nil {
		t.Fatal("clicked the page area, which is not a control")
	}
}

func TestOpeningWaitsForThePage(t *testing.T) {
	quick(t)
	page := &fakePage{snap: flights(), loading: 2}
	s := &Screen{Page: page}
	if err := s.Open(context.Background(), "https://www.example.com"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(page.opened) != 1 || page.loading != 0 {
		t.Errorf("opened %v with %d load check(s) left, want one tab and the load waited out", page.opened, page.loading)
	}
}

func TestActingCarriesOnAsSoonAsThePageAnswers(t *testing.T) {
	quick(t)
	AnswerWait = 5 * time.Second
	page := &fakePage{snap: flights()}
	s := &Screen{Page: page}
	tree, _ := s.Capture(context.Background(), "")
	page.changeOnRead = page.reads + 2

	start := time.Now()
	err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: []uiact.Step{
		{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Search flights"}},
	}}, tree)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("waited %v after the page had already changed", took)
	}
}

func TestASecretFieldIsNeverTypedInto(t *testing.T) {
	quick(t)
	snap := flights()
	snap.Controls = append(snap.Controls, control{Role: "AXSecureTextField", Title: "Card number", Enabled: true, X: 100, Y: 400, W: 200, H: 30})
	page := &fakePage{snap: snap}
	s := &Screen{Page: page}
	tree, _ := s.Capture(context.Background(), "")
	before := len(page.asked)

	err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: []uiact.Step{
		{Do: "type", Target: uiact.Selector{Role: "AXSecureTextField", Title: "Card number"}, Text: "4111111111111111"},
	}}, tree)
	if err == nil {
		t.Fatal("typed into a card number field")
	}
	if len(page.asked) != before {
		t.Error("something was sent to the page for a secret field")
	}
}

func TestAPageThatIgnoresAScriptedClickIsClickedForReal(t *testing.T) {
	quick(t)
	page := &fakePage{snap: flights()}
	var clicked [][2]int
	raised := 0
	s := &Screen{
		Page:      page,
		RealClick: func(x, y int) { clicked = append(clicked, [2]int{x, y}); page.snap.Title = "Results" },
		Raise:     func(context.Context, string) error { raised++; return nil },
	}
	tree, _ := s.Capture(context.Background(), "")

	err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: []uiact.Step{
		{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Search flights"}},
	}}, tree)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if len(clicked) != 1 || clicked[0] != [2]int{160, 315} {
		t.Fatalf("clicked %v, want once in the middle of the button", clicked)
	}
	if raised != 1 {
		t.Errorf("raised the browser %d time(s) before a real click, want 1", raised)
	}
}

func TestAPageThatAnswersIsNotClickedTwice(t *testing.T) {
	quick(t)
	AnswerWait = 5 * time.Second
	page := &fakePage{snap: flights()}
	clicks := 0
	s := &Screen{Page: page, RealClick: func(int, int) { clicks++ }}
	tree, _ := s.Capture(context.Background(), "")
	page.changeOnRead = page.reads + 1

	if err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: []uiact.Step{
		{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Search flights"}},
	}}, tree); err != nil {
		t.Fatalf("Act: %v", err)
	}
	if clicks != 0 {
		t.Errorf("clicked for real %d time(s) on a page that had already answered", clicks)
	}
}

func TestChoosingAFileOnAPageUsesTheRealPointerThenTheChooser(t *testing.T) {
	var clicked [][2]int
	var chosen []string
	page := &fakePage{snap: snapshot{
		Host: "site.example", Ready: "complete",
		Controls: []control{{Role: "AXButton", Title: "Select files", Enabled: true, X: 100, Y: 200, W: 120, H: 40}},
	}}
	s := &Screen{
		Page:      page,
		RealClick: func(x, y int) { clicked = append(clicked, [2]int{x, y}) },
		Choose: func(_ context.Context, app, path string) error {
			chosen = append(chosen, app+" "+path)
			return nil
		},
	}
	quick(t)

	tree, err := s.Capture(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	step := uiact.Step{Do: "choose", Target: uiact.Selector{Role: "AXButton", Title: "Select files"}, Text: "/tmp/clip.mp4"}
	if err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: []uiact.Step{step}}, tree); err != nil {
		t.Fatalf("Act: %v", err)
	}
	if len(clicked) != 1 || clicked[0] != [2]int{160, 220} {
		t.Errorf("clicked = %v, want one real click in the middle of the button", clicked)
	}
	if len(chosen) != 1 || chosen[0] != "Google Chrome /tmp/clip.mp4" {
		t.Errorf("chosen = %q, want the chooser worked in the browser with the path", chosen)
	}
	if n := scriptedActs(page); n != 0 {
		t.Errorf("sent %d scripted act(s); a scripted click cannot open a file chooser", n)
	}
}

func TestCopyAndPasteOnAPageArePressedForReal(t *testing.T) {
	var pressed []int
	page := &fakePage{snap: snapshot{
		Host: "site.example", Ready: "complete",
		Controls: []control{{Role: "AXTextField", Title: "Title", Enabled: true, X: 100, Y: 200, W: 120, H: 40}},
	}}
	s := &Screen{Page: page, RealKey: func(code int, _ uint64) { pressed = append(pressed, code) }}
	quick(t)
	pause := HandPause
	HandPause = time.Millisecond
	t.Cleanup(func() { HandPause = pause })

	tree, _ := s.Capture(context.Background(), "")
	field := uiact.Selector{Role: "AXTextField", Title: "Title"}
	paste := uiact.Step{Do: "press", Key: "V", Modifiers: []string{"Command"}, Target: field}
	down := uiact.Step{Do: "press", Key: "Down", Target: field}
	if err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: []uiact.Step{paste, down}}, tree); err != nil {
		t.Fatalf("Act: %v", err)
	}
	if len(pressed) != 1 || pressed[0] != uiact.Keys["V"] {
		t.Errorf("pressed = %v, want Command-V as a real key", pressed)
	}
	if n := scriptedActs(page); n != 1 {
		t.Errorf("sent %d scripted act(s), want the arrow key sent to the page", n)
	}
}

func scriptedActs(p *fakePage) int {
	n := 0
	for _, script := range p.asked {
		if strings.HasPrefix(script, actOnPage) {
			n++
		}
	}
	return n
}

func TestTextIsPutBackIntoAFieldOnAPage(t *testing.T) {
	page := &fakePage{snap: snapshot{
		Host: "site.example", Ready: "complete",
		Controls: []control{{Role: "AXTextField", Title: "Title", Value: "new", Enabled: true, X: 100, Y: 200, W: 120, H: 40}},
	}}
	s := &Screen{Page: page}
	quick(t)

	if err := s.PutBack(context.Background(), "", uiact.Selector{Role: "AXTextField", Title: "Title"}, "what was there"); err != nil {
		t.Fatalf("PutBack: %v", err)
	}
	sent := ""
	for _, script := range page.asked {
		if strings.HasPrefix(script, actOnPage) {
			sent = script[len(actOnPage):]
		}
	}
	if !strings.Contains(sent, `"verb":"type"`) || !strings.Contains(sent, `"text":"what was there"`) {
		t.Errorf("sent %q, want the old text typed back into the field", sent)
	}

	if err := s.PutBack(context.Background(), "", uiact.Selector{Role: "AXTextField", Title: "Gone"}, "x"); err == nil {
		t.Error("putting text back into a field that is not there was reported as done")
	}
}

func TestTheNewStepsReachAPageInItsOwnTerms(t *testing.T) {
	var dragged []int
	page := &fakePage{snap: snapshot{
		Host: "site.example", Ready: "complete",
		Controls: []control{
			{Role: "AXButton", Title: "Card", Enabled: true, X: 100, Y: 100, W: 100, H: 40},
			{Role: "AXButton", Title: "Done column", Enabled: true, X: 500, Y: 100, W: 200, H: 400},
			{Role: "AXSlider", Title: "Volume", Value: "20", Low: 0, High: 100, Enabled: true, X: 100, Y: 300, W: 200, H: 20},
		},
	}}
	s := &Screen{Page: page, RealDrag: func(x1, y1, x2, y2 int) { dragged = []int{x1, y1, x2, y2} }}
	quick(t)

	tree, err := s.Capture(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if text := tree.TextWithin(4000); !strings.Contains(text, `AXSlider "Volume" value=20 (from 0 to 100)`) {
		t.Errorf("the page's slider is not shown with its range:\n%s", text)
	}

	card := uiact.Selector{Role: "AXButton", Title: "Card"}
	column := uiact.Selector{Role: "AXButton", Title: "Done column"}
	steps := []uiact.Step{
		{Do: "doubleclick", Target: card},
		{Do: "rightclick", Target: card},
		{Do: "scroll", Target: uiact.Selector{Role: WebArea}, Text: "down"},
		{Do: "set", Target: uiact.Selector{Role: "AXSlider", Title: "Volume"}, Text: "60"},
		{Do: "drag", Target: card, To: &column},
	}
	if err := s.Act(context.Background(), uiact.Program{App: tree.App, Steps: steps}, tree); err != nil {
		t.Fatalf("Act: %v", err)
	}

	var sent []string
	for _, script := range page.asked {
		if strings.HasPrefix(script, actOnPage) {
			sent = append(sent, script[len(actOnPage):])
		}
	}
	for i, want := range []string{`"verb":"dblclick"`, `"verb":"contextmenu"`, `"verb":"scrollby","id":-1,"text":"down"`, `"verb":"set"`} {
		if i >= len(sent) || !strings.Contains(sent[i], want) {
			t.Errorf("act %d = %q, want %s", i+1, sent, want)
		}
	}
	if len(sent) != 4 {
		t.Errorf("sent %d scripted act(s), want 4: a drag is made with the real pointer", len(sent))
	}
	if len(dragged) != 4 || dragged[0] != 150 || dragged[1] != 120 || dragged[2] != 600 || dragged[3] != 300 {
		t.Errorf("dragged %v, want from the middle of the card to the middle of the column", dragged)
	}
}
