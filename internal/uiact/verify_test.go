package uiact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func withLookup(t *testing.T, role, title string, owned bool) {
	t.Helper()
	original := lookupAt
	lookupAt = func(int, int, int) (string, string, bool, error) {
		return role, title, owned, nil
	}
	t.Cleanup(func() { lookupAt = original })
}

func withFrontOwner(t *testing.T, pid int, name string, known bool) {
	t.Helper()
	original := frontOwnerAt
	frontOwnerAt = func(int, int) (int, string, bool) { return pid, name, known }
	t.Cleanup(func() { frontOwnerAt = original })
}

func clickSend() Program {
	return Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Send"}},
	}}
}

func TestVerifyAcceptsTheControlItExpects(t *testing.T) {
	withFrontOwner(t, 42, "Telegram", true)
	withLookup(t, "AXButton", "Send", true)
	if err := VerifyTargets(42, clickSend(), window()); err != nil {
		t.Errorf("refused a matching control: %v", err)
	}
}

func TestVerifyRefusesADifferentControl(t *testing.T) {
	withFrontOwner(t, 42, "Telegram", true)
	withLookup(t, "AXButton", "Delete", true)
	err := VerifyTargets(42, clickSend(), window())
	if err == nil {
		t.Fatal("clicked a control that is not the one planned")
	}
	if !strings.Contains(err.Error(), "found a AXButton \"Delete\"") {
		t.Errorf("err = %v", err)
	}
}

// Which window is on top is the question the accessibility hit test cannot
// answer while Stage Manager is running, so it is asked of the window list.
func TestVerifyRefusesWhenAnotherApplicationIsInFront(t *testing.T) {
	withFrontOwner(t, 99, "Code", true)
	withLookup(t, "AXButton", "Send", true)
	err := VerifyTargets(42, clickSend(), window())
	if err == nil {
		t.Fatal("clicked through another application's window")
	}
	if !strings.Contains(err.Error(), "inside Code's window") {
		t.Errorf("err = %v", err)
	}
}

// Stage Manager answers the hit test with its own element rather than the
// application's. That costs the control check, and must not cost the click:
// the window list has already said the point belongs to the right application.
func TestVerifyStillActsWhenTheHitTestIsNotTheApplications(t *testing.T) {
	withFrontOwner(t, 42, "Telegram", true)
	withLookup(t, "AXGroup", "", false)
	if err := VerifyTargets(42, clickSend(), window()); err != nil {
		t.Errorf("refused a click the window list vouched for: %v", err)
	}
}

func TestTreeTextWithholdsValuesExceptWhereTheCursorIs(t *testing.T) {
	tree := Tree{App: "Notes", Window: "Notes", Elements: []Element{
		{Role: "AXTextArea", Title: "Body", Value: "private note contents", Enabled: true},
		{Role: "AXTextField", Title: "Search", Value: "typed query", Enabled: true, Focused: true},
	}}

	got := tree.Text()
	if strings.Contains(got, "private note contents") {
		t.Errorf("the contents of an unfocused control went into the prompt:\n%s", got)
	}
	if !strings.Contains(got, "(holds text)") {
		t.Errorf("a control holding text was not mentioned at all:\n%s", got)
	}
	if !strings.Contains(got, "typed query") {
		t.Errorf("the focused control's value was withheld, which the model needs:\n%s", got)
	}
}

type frontRunner struct{}

func (frontRunner) Run(context.Context, string) ([]byte, error) {
	return []byte(`{"ok":true,"value":"front"}`), nil
}

func withOnScreen(t *testing.T, there bool) {
	t.Helper()
	original := onScreen
	originalWait := windowWait
	onScreen = func(int) bool { return there }
	windowWait = 20 * time.Millisecond
	t.Cleanup(func() {
		onScreen = original
		windowWait = originalWait
	})
}

// Bringing an application forward is only done so its window can be read and
// clicked. Reporting success when the process is frontmost and nothing of it
// is on screen sends the failure somewhere further along, where it arrives as
// the accessibility API being unhelpful rather than a window being hidden.
func TestBringForwardRefusesWhenNoWindowReachesTheScreen(t *testing.T) {
	withOnScreen(t, false)

	err := bringForward(context.Background(), frontRunner{}, "Notes", 42)
	if !errors.Is(err, ErrWindowHidden) {
		t.Fatalf("err = %v, want the hidden-window sentinel", err)
	}
	if !strings.Contains(err.Error(), "Notes") {
		t.Errorf("err = %v, which does not say which application", err)
	}
	if Fix(err) == "" {
		t.Error("a hidden window came with no way to fix it")
	}
}

func TestBringForwardAcceptsAWindowThatIsOnScreen(t *testing.T) {
	withOnScreen(t, true)

	if err := bringForward(context.Background(), frontRunner{}, "Notes", 42); err != nil {
		t.Errorf("refused a window that is on screen: %v", err)
	}
}

type countingRunner struct{ calls int }

func (c *countingRunner) Run(context.Context, string) ([]byte, error) {
	c.calls++
	return []byte(`{"ok":true,"value":"front"}`), nil
}

func withRaise(t *testing.T, err error) {
	t.Helper()
	original := raise
	raise = func(int) error { return err }
	t.Cleanup(func() { raise = original })
}

func withFront(t *testing.T, name string) {
	t.Helper()
	original := askTheWindowList
	askTheWindowList = func() (string, bool) { return name, name != "" }
	t.Cleanup(func() { askTheWindowList = original })
}

// Accessibility is already granted for reading the window. Raising it that way
// costs nothing more; going through System Events costs an Automation grant.
func TestBringForwardRaisesWithoutAskingForAutomation(t *testing.T) {
	withOnScreen(t, true)
	withFront(t, "Something Else")
	withRaise(t, nil)

	r := &countingRunner{}
	if err := bringForward(context.Background(), r, "Notes", 42); err != nil {
		t.Fatalf("bringForward: %v", err)
	}
	if r.calls != 0 {
		t.Errorf("asked System Events %d time(s) when accessibility had already raised it", r.calls)
	}
}

func TestBringForwardFallsBackToEventsWhenAccessibilityCannotRaise(t *testing.T) {
	withOnScreen(t, true)
	withFront(t, "Something Else")
	withRaise(t, errors.New("accessibility error -25211"))

	r := &countingRunner{}
	if err := bringForward(context.Background(), r, "Notes", 42); err != nil {
		t.Fatalf("bringForward: %v", err)
	}
	if r.calls != 1 {
		t.Errorf("fell back %d time(s), want 1", r.calls)
	}
}

func TestBringForwardDoesNothingWhenItIsAlreadyInFront(t *testing.T) {
	withOnScreen(t, true)
	withFront(t, "Notes")
	withRaise(t, errors.New("should not be called"))

	r := &countingRunner{}
	if err := bringForward(context.Background(), r, "Notes", 42); err != nil {
		t.Fatalf("bringForward: %v", err)
	}
	if r.calls != 0 {
		t.Errorf("raised an application that was already in front")
	}
}

func withWalker(t *testing.T, fn func(int, string) (Tree, error)) {
	t.Helper()
	original := walker
	walker = fn
	t.Cleanup(func() { walker = original })
}

func TestCaptureRetriesAfterBringingAppForward(t *testing.T) {
	withOnScreen(t, true)
	withFront(t, "Something Else")
	withRaise(t, nil)

	calls := 0
	expectedTree := Tree{App: "Notes", Window: "Notes"}
	withWalker(t, func(pid int, app string) (Tree, error) {
		calls++
		if calls == 1 {
			return Tree{}, ErrNoWindow
		}
		return expectedTree, nil
	})

	tree, err := Capture(context.Background(), &pidRunner{pid: 42}, "Notes")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if calls != 2 {
		t.Errorf("walked %d times, want 2", calls)
	}
	if tree.App != "Notes" {
		t.Errorf("tree.App = %q, want Notes", tree.App)
	}
}

type pidRunner struct {
	pid int
}

func (p *pidRunner) Run(context.Context, string) ([]byte, error) {
	return []byte(fmt.Sprintf(`{"ok":true,"value":%d}`, p.pid)), nil
}

func withClickPoster(t *testing.T, fn func(int, int)) {
	t.Helper()
	original := clickPoster
	clickPoster = fn
	t.Cleanup(func() { clickPoster = original })
}

func withTextPoster(t *testing.T, fn func(string)) {
	t.Helper()
	original := textPoster
	textPoster = fn
	t.Cleanup(func() { textPoster = original })
}

func withKeyPoster(t *testing.T, fn func(int)) {
	t.Helper()
	original := keyPoster
	keyPoster = fn
	t.Cleanup(func() { keyPoster = original })
}

func TestRunScreenExecutesClickOnTargetControl(t *testing.T) {
	withOnScreen(t, true)
	withFront(t, "Telegram")
	withFrontOwner(t, 42, "Telegram", true)
	withLookup(t, "AXButton", "Send", true)

	var clicks []struct{ x, y int }
	withClickPoster(t, func(x, y int) {
		clicks = append(clicks, struct{ x, y int }{x, y})
	})

	tree := window()
	p := clickSend()

	if err := RunScreen(context.Background(), &pidRunner{pid: 42}, p, tree); err != nil {
		t.Fatalf("RunScreen: %v", err)
	}

	if len(clicks) != 1 {
		t.Fatalf("clicks = %d, want 1", len(clicks))
	}
	if clicks[0].x != 350 || clicks[0].y != 220 {
		t.Errorf("clicked at (%d, %d), want (350, 220)", clicks[0].x, clicks[0].y)
	}
}

func TestRunScreenRefusesClickWhenTargetVerificationFails(t *testing.T) {
	withOnScreen(t, true)
	withFront(t, "Telegram")
	withFrontOwner(t, 42, "Telegram", true)
	withLookup(t, "AXButton", "Delete", true)

	var clicks []struct{ x, y int }
	withClickPoster(t, func(x, y int) {
		clicks = append(clicks, struct{ x, y int }{x, y})
	})

	tree := window()
	p := clickSend()

	err := RunScreen(context.Background(), &pidRunner{pid: 42}, p, tree)
	if err == nil {
		t.Fatal("RunScreen succeeded when target verification should have failed")
	}
	if len(clicks) != 0 {
		t.Errorf("click was posted despite verification failure: %+v", clicks)
	}
}
