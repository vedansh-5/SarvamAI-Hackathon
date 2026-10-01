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
