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
	withFront(t, "Something Else")
	withRaise(t, nil)

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
	withFront(t, "Something Else")
	withRaise(t, nil)

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
	withRaiseFunc(t, func(int) error { return err })
}

func withRaiseFunc(t *testing.T, fn func(int) error) {
	t.Helper()
	original := raise
	raise = fn
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

// The fallback this replaces went through System Events, whose activate() opens
// an application that is not running. VANS does not open applications (D-032), so
// a raise that fails is the end of it: the person is told to open it themselves.
func TestBringForwardWillNotOpenAnApplicationItCannotRaise(t *testing.T) {
	withOnScreen(t, true)
	withFront(t, "Something Else")
	withRaise(t, errors.New("accessibility error -25211"))

	r := &countingRunner{}
	err := bringForward(context.Background(), r, "Notes", 42)
	if !errors.Is(err, ErrCannotBringForward) {
		t.Fatalf("err = %v, want it to refuse rather than open Notes", err)
	}
	if r.calls != 0 {
		t.Errorf("ran %d script(s); the only way left to raise something must be accessibility", r.calls)
	}
	if !strings.Contains(err.Error(), "Notes") {
		t.Errorf("err = %v, which does not say which application to open", err)
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

// Reading is passive. The coach reads twice a second while it waits for someone
// to act, so a read that brought the application back would undo every switch
// away from it.
func TestCaptureDoesNotBringTheApplicationForward(t *testing.T) {
	withOnScreen(t, true)
	withProcessList(t, 42, true)
	withFront(t, "Something Else")

	raises := 0
	withRaiseFunc(t, func(int) error {
		raises++
		return nil
	})
	withWalker(t, func(int, string) (Tree, error) { return Tree{}, ErrNoWindow })

	if _, err := Capture(context.Background(), &pidRunner{pid: 42}, "Notes"); err == nil {
		t.Fatal("a window that published nothing was reported as read")
	}
	if raises != 0 {
		t.Errorf("raised the application %d time(s) from inside a read", raises)
	}
}

// The process list is the real one unless a test says otherwise, so a test that
// invents a process id has to say so or the machine answers instead.
func withProcessList(t *testing.T, pid int, found bool) {
	t.Helper()
	original := askTheProcessList
	askTheProcessList = func(context.Context, string) (int, bool) { return pid, found }
	t.Cleanup(func() { askTheProcessList = original })
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

func withKeyPoster(t *testing.T, fn func(int, uint64)) {
	t.Helper()
	original := keyPoster
	keyPoster = fn
	t.Cleanup(func() { keyPoster = original })
}

func TestRunScreenExecutesClickOnTargetControl(t *testing.T) {
	withOnScreen(t, true)
	withProcessList(t, 42, true)
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
	withProcessList(t, 42, true)
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

// A step that names a key combination has to arrive as one keystroke with the
// modifiers held, not as a bare key with the modifiers dropped on the floor.
func TestRunScreenHoldsTheModifiersDownForAKeyCombination(t *testing.T) {
	withOnScreen(t, true)
	withProcessList(t, 42, true)
	withFront(t, "Telegram")
	withFrontOwner(t, 42, "Telegram", true)
	withLookup(t, "AXTextArea", "Message", true)

	var pressed []struct {
		code  int
		flags uint64
	}
	withKeyPoster(t, func(code int, flags uint64) {
		pressed = append(pressed, struct {
			code  int
			flags uint64
		}{code, flags})
	})

	p := Program{App: "Telegram", Steps: []Step{{
		Do:        "press",
		Target:    Selector{Role: "AXTextArea", Title: "Message"},
		Key:       "Enter",
		Modifiers: []string{"Command", "Shift"},
	}}}

	if err := RunScreen(context.Background(), &pidRunner{pid: 42}, p, window()); err != nil {
		t.Fatalf("RunScreen: %v", err)
	}
	if len(pressed) != 1 {
		t.Fatalf("posted %d keystroke(s), want 1", len(pressed))
	}
	if pressed[0].code != Keys["Enter"] {
		t.Errorf("key code = %d, want %d", pressed[0].code, Keys["Enter"])
	}
	want := Modifiers["Command"] | Modifiers["Shift"]
	if pressed[0].flags != want {
		t.Errorf("flags = %#x, want %#x", pressed[0].flags, want)
	}
}

func TestValidateRefusesAModifierVANSCannotHold(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{{
		Do:        "press",
		Target:    Selector{Role: "AXTextArea", Title: "Message"},
		Key:       "Enter",
		Modifiers: []string{"Hyper"},
	}}}

	err := Validate(p, window())
	if err == nil {
		t.Fatal("a modifier that does not exist was accepted")
	}
	if got := strings.Join(reasons(t, err), "; "); !strings.Contains(got, "Hyper") {
		t.Errorf("reasons = %q, want the unknown modifier named", got)
	}
}

func TestAKeyCombinationReadsBackTheWayAPersonSaysIt(t *testing.T) {
	step := Step{Do: "press", Key: "S", Modifiers: []string{"Command"}}
	if got := step.Chord(); got != "Command-S" {
		t.Errorf("Chord() = %q, want %q", got, "Command-S")
	}
	if got := (Step{Do: "press", Key: "Enter"}).Chord(); got != "Enter" {
		t.Errorf("Chord() = %q, want %q for a key with nothing held", got, "Enter")
	}
}

// The application has to be named, because the whole answer is "open this one".
func TestARefusedRaiseNamesTheApplication(t *testing.T) {
	withOnScreen(t, true)
	withFront(t, "Something Else")
	withRaise(t, errors.New("accessibility error -25211"))

	err := bringForward(context.Background(), frontRunner{}, "System Settings", 42)

	var cannot *CannotRaiseError
	if !errors.As(err, &cannot) {
		t.Fatalf("err = %v, want a CannotRaiseError", err)
	}
	if cannot.App != "System Settings" {
		t.Errorf("App = %q, want System Settings", cannot.App)
	}
	if !errors.Is(err, ErrCannotBringForward) {
		t.Error("the typed error does not match the sentinel, so callers cannot test for it")
	}
}
