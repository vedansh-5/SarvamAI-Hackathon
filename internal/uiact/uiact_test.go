package uiact

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func window() Tree {
	return Tree{
		App:    "Telegram",
		Window: "Telegram",
		Elements: []Element{
			{Role: "AXTextArea", Title: "Message", Enabled: true,
				Position: []int{100, 200}, Size: []int{200, 40}, Path: []int{0, 1}},
			{Role: "AXButton", Title: "Send", Enabled: true, Actions: []string{"AXPress"},
				Position: []int{320, 200}, Size: []int{60, 40}, Path: []int{0, 2}},
			{Role: "AXButton", Title: "Open", Enabled: true, Actions: []string{"AXPress"},
				Position: []int{10, 300}, Size: []int{50, 20}, Path: []int{1, 0}},
			{Role: "AXButton", Title: "Open", Enabled: true, Actions: []string{"AXPress"},
				Position: []int{10, 330}, Size: []int{50, 20}, Path: []int{1, 1}},
			{Role: "AXStaticText", Title: "Delivered", Enabled: true,
				Position: []int{10, 360}, Size: []int{80, 16}, Path: []int{2}},
			{Role: "AXButton", Title: "Archive", Enabled: false, Actions: []string{"AXPress"},
				Position: []int{10, 380}, Size: []int{50, 20}, Path: []int{3}},
		},
	}
}

func send() Program {
	return Program{
		App: "Telegram",
		Steps: []Step{
			{Do: "type", Target: Selector{Role: "AXTextArea", Title: "Message"}, Text: "on my way"},
			{Do: "click", Target: Selector{Role: "AXButton", Title: "Send"}},
		},
	}
}

func reasons(t *testing.T, err error) []string {
	t.Helper()
	var invalid *InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidError", err)
	}
	return invalid.Reasons
}

func clicked(acts []Act, x, y int) bool {
	for _, a := range acts {
		if a.Do == "click" && a.X == x && a.Y == y {
			return true
		}
	}
	return false
}

func typed(acts []Act) []string {
	var out []string
	for _, a := range acts {
		if a.Do == "type" {
			out = append(out, a.Text)
		}
	}
	return out
}

func mentions(rs []string, want string) bool {
	for _, r := range rs {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}

func TestAValidProgramPlans(t *testing.T) {
	acts, err := Plan(send(), window())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !clicked(acts, 200, 220) {
		t.Errorf("typing step did not click the message box: %+v", acts)
	}
	if got := typed(acts); len(got) != 1 || got[0] != "on my way" {
		t.Errorf("typing step did not type: %+v", acts)
	}
	if !clicked(acts, 350, 220) {
		t.Errorf("send step did not click Send: %+v", acts)
	}
}

func TestAmbiguityIsRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Open"}},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "does not say which one") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestIndexResolvesAmbiguity(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Open", Index: 2}},
	}}
	acts, err := Plan(p, window())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !clicked(acts, 35, 340) {
		t.Errorf("index picked the wrong control: %+v", acts)
	}
}

func TestTypingIntoSomethingThatCannotHoldTextIsRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXStaticText", Title: "Delivered"}, Text: "hello"},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "cannot hold typed text") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestADisabledElementIsRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Archive"}},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "is disabled") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestAnElementThatIsNotThereIsRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Publish"}},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "nothing on screen is a AXButton") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestUnknownVerbAndKeyAreRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "drag", Target: Selector{Role: "AXButton", Title: "Send"}},
		{Do: "press", Target: Selector{Role: "AXTextArea", Title: "Message"}, Key: "F13"},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "not something VANS does") || !mentions(rs, "not a key VANS presses") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestTooManyStepsAreRefused(t *testing.T) {
	p := Program{App: "Telegram"}
	for i := 0; i < MaxSteps+1; i++ {
		p.Steps = append(p.Steps, Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Send"}})
	}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "is the most a single request may take") {
		t.Errorf("reasons = %v", rs)
	}
}

// Text is carried as data and posted as key events, so there is no script for
// it to break out of. It arrives exactly as it was written, quotes and all.
func TestTextIsCarriedAsDataNotAsScript(t *testing.T) {
	const hostile = `"); se.keystroke("owned"); ("`
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXTextArea", Title: "Message"}, Text: hostile},
	}}
	acts, err := Plan(p, window())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	got := typed(acts)
	if len(got) != 1 || got[0] != hostile {
		t.Errorf("text did not survive as written: %+v", acts)
	}
}

func TestBakedCatchesAValueWrittenIn(t *testing.T) {
	p := send()
	got := Baked(p, map[string]string{"s0": "on my way"})
	if len(got) != 1 || got[0] != "on my way" {
		t.Errorf("Baked = %v", got)
	}
}

func TestFillReplacesTextAndTitle(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXTextArea", Title: "{s1}"}, Text: "{s0}"},
	}}
	got := p.Fill(func(v string) string {
		switch v {
		case "{s0}":
			return "hello"
		case "{s1}":
			return "Message"
		}
		return v
	})
	if got.Steps[0].Text != "hello" || got.Steps[0].Target.Title != "Message" {
		t.Errorf("Fill = %+v", got.Steps[0])
	}
}

func TestTreeTextNumbersDuplicates(t *testing.T) {
	got := window().Text()
	if !strings.Contains(got, "(index 1 of 2)") || !strings.Contains(got, "(index 2 of 2)") {
		t.Errorf("duplicates not distinguishable:\n%s", got)
	}
	if !strings.Contains(got, "[disabled]") {
		t.Errorf("disabled element not marked:\n%s", got)
	}
}

// The claim that makes this tier cacheable: the program names a role and a
// title, so it still finds its control when the window has been rearranged
// and every path has changed.
func TestAProgramFollowsItsControlsWhenTheyMove(t *testing.T) {
	p := send()
	if err := Validate(p, window()); err != nil {
		t.Fatalf("the program did not fit the window it was written for: %v", err)
	}

	moved := window()
	for i := range moved.Elements {
		if len(moved.Elements[i].Position) == 2 {
			moved.Elements[i].Position[1] += 300
		}
	}

	acts, err := Plan(p, moved)
	if err != nil {
		t.Fatalf("Plan against the moved window: %v", err)
	}
	if !clicked(acts, 200, 520) {
		t.Errorf("typing did not follow its control: %+v", acts)
	}
	if !clicked(acts, 350, 520) {
		t.Errorf("the click did not follow its control: %+v", acts)
	}
}

func TestAProgramIsRefusedWhenItsControlIsGone(t *testing.T) {
	gone := window()
	gone.Elements = gone.Elements[1:]

	if err := Validate(send(), gone); err == nil {
		t.Fatal("a program whose control disappeared was accepted")
	}
}

func typingTree(controls ...Element) Tree {
	return Tree{App: "Editor", Window: "Editor", Elements: controls}
}

func TestTypingTargetPrefersTheControlWithTheCursor(t *testing.T) {
	tree := typingTree(
		Element{Role: "AXTextField", Title: "Search", Enabled: true, Path: []int{0}},
		Element{Role: "AXTextArea", Title: "Body", Enabled: true, Focused: true, Path: []int{1}},
	)
	got, why := tree.TypingTarget()
	if why != "" {
		t.Fatalf("refused a window with a cursor in it: %s", why)
	}
	if got.Title != "Body" {
		t.Errorf("picked %q, want the control holding the cursor", got.Title)
	}
}

func TestTypingTargetTakesTheOnlyCandidate(t *testing.T) {
	tree := typingTree(
		Element{Role: "AXTextArea", Title: "Body", Enabled: true, Path: []int{0}},
		Element{Role: "AXButton", Title: "Send", Enabled: true, Path: []int{1}},
	)
	got, why := tree.TypingTarget()
	if why != "" || got.Title != "Body" {
		t.Errorf("got %+v, why %q", got, why)
	}
}

// Two places the text could go and no cursor is exactly the case a
// confirmation cannot catch: the preview names one and the other is filled.
func TestTypingTargetRefusesWhenSeveralCouldBeMeant(t *testing.T) {
	tree := typingTree(
		Element{Role: "AXTextField", Title: "First", Enabled: true, Path: []int{0}},
		Element{Role: "AXTextField", Title: "Second", Enabled: true, Path: []int{1}},
	)
	if _, why := tree.TypingTarget(); why == "" {
		t.Fatal("two candidates and no cursor was accepted")
	}
}

func TestTypingTargetIgnoresWhatCannotBeTypedInto(t *testing.T) {
	tree := typingTree(
		Element{Role: "AXTextField", Title: "Locked", Enabled: false, Path: []int{0}},
		Element{Role: "AXTextArea", Title: "Body", Enabled: true, Path: []int{1}},
	)
	got, why := tree.TypingTarget()
	if why != "" || got.Title != "Body" {
		t.Errorf("got %+v, why %q", got, why)
	}
}

func TestTypingTargetSaysWhenNothingCanHoldText(t *testing.T) {
	tree := typingTree(Element{Role: "AXButton", Title: "Send", Enabled: true, Path: []int{0}})
	if _, why := tree.TypingTarget(); why == "" {
		t.Fatal("a window with no text control was accepted")
	}
}

// A selector is only numbered when the number is what distinguishes it.
func TestSelectorForNumbersOnlyDuplicates(t *testing.T) {
	w := window()

	var lastOpen, send Element
	for _, el := range w.Elements {
		if el.Title == "Open" {
			lastOpen = el
		}
		if el.Title == "Send" {
			send = el
		}
	}

	if got := w.SelectorFor(lastOpen); got.Index != 2 {
		t.Errorf("second duplicate got index %d, want 2", got.Index)
	}
	if got := w.SelectorFor(send); got.Index != 0 {
		t.Errorf("a unique control was numbered: %+v", got)
	}

	p := Program{App: w.App, Steps: []Step{{Do: "click", Target: w.SelectorFor(lastOpen)}}}
	if err := Validate(p, w); err != nil {
		t.Errorf("a selector built for a duplicate did not resolve: %v", err)
	}
}

const notRunning = "VansNoSuchApplication"

type replyRunner string

func (r replyRunner) Run(context.Context, string) ([]byte, error) { return []byte(r), nil }

func withoutTheWindowList(t *testing.T) {
	t.Helper()
	original := askTheWindowList
	askTheWindowList = func() (string, bool) { return "", false }
	t.Cleanup(func() { askTheWindowList = original })
}

func TestFrontmostReadsTheName(t *testing.T) {
	withoutTheWindowList(t)
	got, err := Frontmost(context.Background(), replyRunner(`{"ok":true,"value":"Notes"}`))
	if err != nil {
		t.Fatalf("Frontmost: %v", err)
	}
	if got != "Notes" {
		t.Errorf("got %q, want Notes", got)
	}
}

func TestFrontmostSaysWhenNothingIsInFront(t *testing.T) {
	withoutTheWindowList(t)
	_, err := Frontmost(context.Background(),
		replyRunner(`{"ok":false,"error":"no application is in front"}`))
	if err == nil {
		t.Fatal("an empty answer was reported as success")
	}
}

// The permission is the one failure with a fix the user can act on, so it has
// to survive as itself rather than as a generic refusal.
func TestFrontmostReportsTheMissingPermission(t *testing.T) {
	withoutTheWindowList(t)
	_, err := Frontmost(context.Background(),
		replyRunner(`{"ok":false,"error":"osascript is not allowed assistive access."}`))
	if !errors.Is(err, ErrNoAssistiveAccess) {
		t.Fatalf("err = %v, want the assistive access sentinel", err)
	}
	if Fix(err) == "" {
		t.Error("the permission error carries no fix")
	}
}

func TestCaptureReportsTheMissingPermission(t *testing.T) {
	_, err := Capture(context.Background(),
		replyRunner(`{"ok":false,"error":"osascript is not allowed assistive access."}`), notRunning)
	if !errors.Is(err, ErrNoAssistiveAccess) {
		t.Fatalf("err = %v, want the assistive access sentinel", err)
	}
}

func TestCaptureReportsAWindowlessApplication(t *testing.T) {
	_, err := Capture(context.Background(),
		replyRunner(`{"ok":false,"error":"that application has no window open"}`), notRunning)
	if !errors.Is(err, ErrNoWindow) {
		t.Fatalf("err = %v, want the no-window sentinel", err)
	}
}

func placed(role, title string, x, y, w, h int) Element {
	return Element{
		Role: role, Title: title, Enabled: true,
		Position: []int{x, y}, Size: []int{w, h}, Path: []int{0},
	}
}

func TestCenterIsTheMiddleOfTheControl(t *testing.T) {
	x, y, ok := placed("AXButton", "Send", 100, 200, 80, 40).Center()
	if !ok || x != 140 || y != 220 {
		t.Errorf("centre = (%d,%d) ok=%v, want (140,220)", x, y, ok)
	}
}

func TestCenterRefusesAControlWithNoBox(t *testing.T) {
	if _, _, ok := (Element{Role: "AXButton"}).Center(); ok {
		t.Error("a control with no position reported a centre")
	}
	zero := placed("AXButton", "Send", 10, 10, 0, 0)
	if _, _, ok := zero.Center(); ok {
		t.Error("a control with no size reported a centre")
	}
}

func TestRenderScreenClicksWhereTheControlIs(t *testing.T) {
	tree := Tree{App: "Editor", Window: "Editor", Elements: []Element{
		placed("AXTextArea", "Message", 100, 200, 80, 40),
	}}
	tree.Elements[0].Path = []int{0, 1}

	p := Program{App: "Editor", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXTextArea", Title: "Message"}, Text: "hello"},
	}}

	acts, err := Plan(p, tree)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !clicked(acts, 140, 220) {
		t.Errorf("did not click the control's centre: %+v", acts)
	}
	if got := typed(acts); len(got) != 1 || got[0] != "hello" {
		t.Errorf("did not type the text: %+v", acts)
	}
}

func TestRenderScreenRefusesAControlItCannotSee(t *testing.T) {
	tree := Tree{App: "Editor", Window: "Editor", Elements: []Element{
		{Role: "AXTextArea", Title: "Message", Enabled: true, Path: []int{0}},
	}}
	p := Program{App: "Editor", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXTextArea", Title: "Message"}},
	}}

	if _, err := Plan(p, tree); err == nil {
		t.Fatal("a control with no position on screen was clicked anyway")
	}
}

func TestFirstPIDTakesTheFirstOne(t *testing.T) {
	pid, err := firstPID([]byte("4321\n9876\n"))
	if err != nil || pid != 4321 {
		t.Errorf("pid = %d, err = %v", pid, err)
	}
}

func TestFirstPIDRefusesNothingUsable(t *testing.T) {
	if _, err := firstPID([]byte("   \n")); err == nil {
		t.Error("empty output produced a process id")
	}
	if _, err := firstPID([]byte("not-a-pid\n")); err == nil {
		t.Error("garbage produced a process id")
	}
}

func withDisplay(t *testing.T, b box) {
	t.Helper()
	original := displays
	displays = func() (box, bool) { return b, true }
	t.Cleanup(func() { displays = original })
}

func notesTree() Tree {
	return Tree{App: "Notes", Window: "Notes", Elements: []Element{
		{Role: "AXSplitGroup", Enabled: true,
			Position: []int{202, 131}, Size: []int{1000, 660}, Path: []int{0}},
		{Role: "AXTextArea", Enabled: true,
			Position: []int{711, 167}, Size: []int{474, 22468}, Path: []int{0, 5, 0}},
	}}
}

// A note longer than the window makes the text area 22468pt tall, so its
// middle is thousands of points below the screen. The click has to land in the
// part that is actually showing.
func TestClickPointStaysInsideTheVisiblePartOfATallControl(t *testing.T) {
	withDisplay(t, box{0, 0, 1710, 1112})

	tree := notesTree()
	x, y, ok := tree.ClickPoint(tree.Elements[1])
	if !ok {
		t.Fatal("a control that is on screen reported no click point")
	}
	if x != 948 || y != 479 {
		t.Errorf("click point = (%d,%d), want (948,479)", x, y)
	}
}

func TestClickPointRefusesAControlScrolledOutOfView(t *testing.T) {
	withDisplay(t, box{0, 0, 1710, 1112})

	tree := notesTree()
	tree.Elements[1].Position = []int{711, -5000}
	tree.Elements[1].Size = []int{474, 1000}

	if _, _, ok := tree.ClickPoint(tree.Elements[1]); ok {
		t.Error("a control scrolled above the window reported a click point")
	}
}

func TestClickPointClipsToTheDisplay(t *testing.T) {
	withDisplay(t, box{0, 0, 1710, 1112})

	tree := Tree{App: "Editor", Window: "Editor", Elements: []Element{
		{Role: "AXTextArea", Title: "Message", Enabled: true,
			Position: []int{1600, 100}, Size: []int{400, 40}, Path: []int{0}},
	}}

	x, _, ok := tree.ClickPoint(tree.Elements[0])
	if !ok {
		t.Fatal("a control half on screen reported no click point")
	}
	if x != 1655 {
		t.Errorf("x = %d, want 1655 — the middle of the part on screen", x)
	}
}

func TestRenderScreenClicksTheVisiblePartOfATallControl(t *testing.T) {
	withDisplay(t, box{0, 0, 1710, 1112})

	tree := notesTree()
	p := Program{App: "Notes", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXTextArea"}, Text: "hello"},
	}}

	acts, err := Plan(p, tree)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !clicked(acts, 948, 479) {
		t.Errorf("did not click the visible part: %+v", acts)
	}
}

// The window list answers without an Automation grant, so it is asked first and
// the script is never reached when it can.
func TestFrontmostPrefersTheWindowList(t *testing.T) {
	original := askTheWindowList
	askTheWindowList = func() (string, bool) { return "Notes", true }
	t.Cleanup(func() { askTheWindowList = original })

	got, err := Frontmost(context.Background(), replyRunner(`{"ok":true,"value":"Something Else"}`))
	if err != nil {
		t.Fatalf("Frontmost: %v", err)
	}
	if got != "Notes" {
		t.Errorf("got %q, want the window list's answer", got)
	}
}
