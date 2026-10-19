package eyes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func readWords(words ...uiact.Word) func([]byte) ([]uiact.Word, error) {
	return func([]byte) ([]uiact.Word, error) { return words, nil }
}

var toolbar = []uiact.Word{
	{Text: "Export", X: 0.905, Y: 0.05, W: 0.06, H: 0.04},
	{Text: "Cut", X: 0.10, Y: 0.30, W: 0.03, H: 0.04},
	{Text: "Copy", X: 0.20, Y: 0.30, W: 0.04, H: 0.04},
	{Text: "Paste", X: 0.30, Y: 0.30, W: 0.05, H: 0.04},
	{Text: "Render Queue", X: 0.50, Y: 0.30, W: 0.11, H: 0.04},
}

func element(t *testing.T, tree uiact.Tree, role, title string) uiact.Element {
	t.Helper()
	found := tree.Find(uiact.Selector{Role: role, Title: title})
	if len(found) != 1 {
		t.Fatalf("%s %q found %d time(s) in %+v", role, title, len(found), tree.Elements)
	}
	return found[0]
}

func TestWordsOnScreenAreExactAndAButtonTheModelSawTakesTheirPlace(t *testing.T) {
	quick(t)
	s := &Screen{Camera: &camera{jpeg: picture(t)}, Reader: &reader{}, Words: readWords(toolbar[:1]...)}
	tree, err := s.Capture(context.Background(), "Resolve")
	if err != nil {
		t.Fatal(err)
	}
	export := element(t, tree, "AXButton", "Export")
	if export.Estimated || export.Position[0] != 824 || export.Position[1] != 75 {
		t.Errorf("Export = %+v, want the word's exact place at (824, 75), not the model's guess", export)
	}
	search := element(t, tree, "AXTextField", "Search")
	if !search.Estimated {
		t.Errorf("a field the words did not confirm was not marked estimated: %+v", search)
	}
}

type slowReader struct {
	release chan struct{}
	looks   int
}

func (r *slowReader) SeeScreen(context.Context, synth.SeeRequest) (synth.SeeAnswer, error) {
	r.looks++
	<-r.release
	return synth.SeeAnswer{Controls: []synth.Seen{{Kind: "button", Name: "Play", X0: 480, Y0: 900, X1: 520, Y1: 960}}}, nil
}

func TestWithEnoughWordsTheModelLooksWithoutMakingAnyoneWait(t *testing.T) {
	quick(t)
	r := &slowReader{release: make(chan struct{})}
	s := &Screen{Camera: &camera{jpeg: picture(t)}, Reader: r, Words: readWords(toolbar...)}
	ctx := context.Background()

	done := make(chan uiact.Tree)
	go func() {
		tree, _ := s.Capture(ctx, "Resolve")
		done <- tree
	}()
	select {
	case tree := <-done:
		element(t, tree, "AXStaticText", "Render Queue")
	case <-time.After(time.Second):
		t.Fatal("waited for the model although the words were enough to go on")
	}

	close(r.release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		tree, _ := s.Capture(ctx, "Resolve")
		if found := tree.Find(uiact.Selector{Role: "AXButton", Title: "Play"}); len(found) == 1 {
			if !found[0].Estimated {
				t.Errorf("an icon with no words was not marked estimated")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the icon the model found never joined the words")
}

func TestTheMenusTheApplicationPublishesAreIncluded(t *testing.T) {
	quick(t)
	menu := uiact.Element{Role: "AXMenuBarItem", Title: "Timeline", Enabled: true, Position: []int{300, 0}, Size: []int{60, 24}, Path: []int{uiact.MenuBarRoot, 3}}
	s := &Screen{
		Camera: &camera{jpeg: picture(t)},
		Words:  readWords(toolbar...),
		Menus:  func(context.Context, string) ([]uiact.Element, error) { return []uiact.Element{menu}, nil },
	}
	tree, err := s.Capture(context.Background(), "Resolve")
	if err != nil {
		t.Fatal(err)
	}
	if got := element(t, tree, "AXMenuBarItem", "Timeline"); got.Estimated {
		t.Errorf("a published menu was marked estimated")
	}
}

func TestAnEstimatedControlIsNeverClicked(t *testing.T) {
	tree := uiact.Tree{App: "Resolve", Elements: []uiact.Element{
		{Role: "AXWindow", Enabled: true, Position: []int{0, 0}, Size: []int{1000, 800}, Path: []int{}},
		{Role: "AXButton", Title: "Play", Enabled: true, Position: []int{480, 700}, Size: []int{40, 40}, Path: []int{0}, Estimated: true},
	}}
	err := uiact.RunByPointer(context.Background(), nil, uiact.Program{App: "Resolve", Steps: []uiact.Step{
		{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Play"}},
	}}, tree)
	if !errors.Is(err, uiact.ErrEstimated) {
		t.Fatalf("err = %v, want a refusal to click a guessed place", err)
	}
}
