package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/capability"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type twoApps struct {
	trees   map[string]uiact.Tree
	open    []string
	raised  []string
	acts    []string
	reads   int
	current string
}

func (s *twoApps) Capture(_ context.Context, app string) (uiact.Tree, error) {
	s.reads++
	tree, ok := s.trees[app]
	if !ok {
		return uiact.Tree{}, &uiact.NotRunningError{App: app}
	}
	tree.Elements = append([]uiact.Element{}, tree.Elements...)
	tree.Elements = append(tree.Elements, uiact.Element{
		Role: "AXStaticText", Title: strings.Repeat("·", s.reads), Enabled: true,
		Position: []int{0, 0}, Size: []int{10, 10}, Path: []int{9, s.reads},
	})
	return tree, nil
}

func (s *twoApps) Act(_ context.Context, p uiact.Program, t uiact.Tree) error {
	if err := uiact.Validate(p, t); err != nil {
		return err
	}
	s.acts = append(s.acts, t.App+": "+p.Steps[0].Do+" "+p.Steps[0].Target.Title)
	return nil
}

func (s *twoApps) BringToFront(_ context.Context, app string) error {
	s.raised = append(s.raised, app)
	s.current = app
	return nil
}

func (s *twoApps) Frontmost(context.Context) (string, error) { return s.current, nil }
func (s *twoApps) Type(context.Context, string) error        { return nil }
func (s *twoApps) Running() []string                         { return s.open }

func notesAndChat() *twoApps {
	return &twoApps{
		current: "Notes",
		open:    []string{"Notes", "Chat"},
		trees: map[string]uiact.Tree{
			"Notes": {App: "Notes", Window: "Notes", Elements: []uiact.Element{
				{Role: "AXTextArea", Title: "Note", Enabled: true, Position: []int{100, 100}, Size: []int{300, 200}, Path: []int{0}},
			}},
			"Chat": {App: "Chat", Window: "Chat", Elements: []uiact.Element{
				{Role: "AXTextArea", Title: "Reply", Enabled: true, Position: []int{100, 400}, Size: []int{300, 40}, Path: []int{0}},
			}},
		},
	}
}

func note() uiact.Selector  { return uiact.Selector{Role: "AXTextArea", Title: "Note"} }
func reply() uiact.Selector { return uiact.Selector{Role: "AXTextArea", Title: "Reply"} }

func copyThenPaste() []synth.StepAnswer {
	return []synth.StepAnswer{
		{Step: uiact.Step{Do: "press", Key: "A", Modifiers: []string{"Command"}, Target: note(), Describe: "Select the whole note"}},
		{Step: uiact.Step{Do: "press", Key: "C", Modifiers: []string{"Command"}, Target: note(), Describe: "Copy it"}},
		{Step: uiact.Step{Do: "switch", App: "Chat", Target: uiact.Selector{Role: "AXApplication"}, Describe: "Switch to Chat"}},
		{Step: uiact.Step{Do: "click", Target: reply(), Describe: "Click the reply box"}},
		{Step: uiact.Step{Do: "press", Key: "V", Modifiers: []string{"Command"}, Target: reply(), Describe: "Paste it"}},
	}
}

func TestATaskCanMoveFromOneApplicationToAnother(t *testing.T) {
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })

	screen := notesAndChat()
	steps := &scriptedSteps{answers: copyThenPaste()}
	e := &Engine{UIScreen: screen, Steps: steps}

	receipt, err := e.runDrive(context.Background(), &Plan{Drive: &DrivePlan{App: "Notes", Task: "copy this note into Chat"}})
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if receipt.Work == nil || receipt.Work.Steps != 5 {
		t.Fatalf("work = %+v, want all five steps", receipt.Work)
	}

	want := []string{"Notes: press Note", "Notes: press Note", "Chat: click Reply", "Chat: press Reply"}
	if strings.Join(screen.acts, " | ") != strings.Join(want, " | ") {
		t.Errorf("acts = %q\nwant   %q", screen.acts, want)
	}
	if len(screen.raised) == 0 || screen.raised[len(screen.raised)-1] != "Chat" {
		t.Errorf("raised = %q, want Chat brought to the front for the second half", screen.raised)
	}
}

func TestSwitchingToAnApplicationThatIsNotOpenStopsAndSaysSo(t *testing.T) {
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })

	screen := notesAndChat()
	screen.open = []string{"Notes"}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{answers: copyThenPaste()}}

	_, err := e.runDrive(context.Background(), &Plan{Drive: &DrivePlan{App: "Notes", Task: "copy this note into Chat"}})
	var broke *BrokeOffError
	if !errors.As(err, &broke) {
		t.Fatalf("err = %v, want the run stopped partway so it can be carried on", err)
	}
	if !strings.Contains(err.Error(), "Chat") || !strings.Contains(err.Error(), "not open") {
		t.Errorf("err = %v, want it to say Chat is not open", err)
	}
	for _, raised := range screen.raised {
		if raised == "Chat" {
			t.Error("tried to bring forward an application that is not open")
		}
	}
}

func TestSwitchingIntoAForbiddenApplicationIsRefused(t *testing.T) {
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })

	screen := notesAndChat()
	e := &Engine{UIScreen: screen, Steps: &alwaysStep{step: copyThenPaste()[2].Step}, Rulebook: rulebook{Never: []string{"Chat"}}}

	_, err := e.runDrive(context.Background(), &Plan{Drive: &DrivePlan{App: "Notes", Task: "copy this note into Chat"}})
	if err == nil {
		t.Fatal("a run was allowed to switch into an application the rules forbid")
	}
	if len(screen.acts) != 0 || len(screen.raised) > 1 {
		t.Errorf("acts = %q, raised = %q; nothing should have happened in Chat", screen.acts, screen.raised)
	}
}

func TestOnlyAFileThePersonNamedMayBeChosen(t *testing.T) {
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(clip, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(secret, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	said := "upload " + clip + " to youtube"

	if got, why := fileToChoose(clip, said); why != "" || got != clip {
		t.Errorf("got %q, %q; want the named file accepted", got, why)
	}
	if _, why := fileToChoose(secret, said); !strings.Contains(why, "never named") {
		t.Errorf("why = %q, want a file nobody named refused", why)
	}
	if _, why := fileToChoose(filepath.Join(dir, "gone.mp4"), "upload gone.mp4"); !strings.Contains(why, "no file at") {
		t.Errorf("why = %q, want a missing file refused", why)
	}
	if _, why := fileToChoose(dir, "upload "+filepath.Base(dir)); !strings.Contains(why, "folder") {
		t.Errorf("why = %q, want a folder refused", why)
	}
	if _, why := fileToChoose("clip.mp4", said); !strings.Contains(why, "full path") {
		t.Errorf("why = %q, want a bare name refused", why)
	}
}

func TestAFileNamedOnlyByItsNameIsFoundAtThePathTheModelGives(t *testing.T) {
	dir := t.TempDir()
	cv := filepath.Join(dir, "Resume.pdf")
	if err := os.WriteFile(cv, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, why := fileToChoose(cv, "upload resume.pdf from my desktop"); why != "" || got != cv {
		t.Errorf("got %q, %q; the person named the file, in a different case", got, why)
	}
}

func TestARequestNamingTwoApplicationsIsSeenAsGoingAcross(t *testing.T) {
	e := &Engine{Apps: capability.New()}

	if got := e.appsNamedIn("copy this from Notes into Safari"); len(got) != 2 {
		t.Errorf("named = %q, want Notes and Safari", got)
	}
	for _, one := range []string{"send this note to Joe", "make a new note in Notes", "how do I change the wallpaper", "open my photos and maps"} {
		if e.spansApps(one) {
			t.Errorf("%q was taken to go across two applications: %q", one, e.appsNamedIn(one))
		}
	}
}

type browserWindow struct {
	fakeScreen
	chooser bool
	pressed []int
}

func (b *browserWindow) Capture(context.Context, string) (uiact.Tree, error) {
	if b.chooser {
		return uiact.Tree{App: "Google Chrome", Window: "Open"}, nil
	}
	return uiact.Tree{App: "Google Chrome", Window: "Site"}, nil
}

func (b *browserWindow) PressKey(code int, _ uint64) {
	b.pressed = append(b.pressed, code)
	b.chooser = false
}

func TestAChooserOpenedByAStrayClickIsClosedAndTheModelTold(t *testing.T) {
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })

	native := &browserWindow{chooser: true}
	page := &fakeScreen{tree: editorTree(), front: "Google Chrome"}
	steps := &recordingSteps{answers: []synth.StepAnswer{{Step: clickMessage()}, {Done: true, Why: "stopping here"}}}
	e := (&Engine{UIScreen: native, PageScreen: page, Steps: steps}).onThePage()

	if _, err := e.runDrive(context.Background(), &Plan{Drive: &DrivePlan{App: "Editor", Task: "upload a video", Web: true, Browser: "Google Chrome"}}); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(native.pressed) != 1 || native.pressed[0] != escapeKey {
		t.Errorf("pressed = %v, want Escape once to close the chooser", native.pressed)
	}
	if len(steps.requests) < 2 || !strings.Contains(strings.Join(steps.requests[1].Done, "\n"), "file chooser") {
		t.Errorf("the model was not told a chooser had opened and been closed: %+v", steps.requests)
	}
}

func TestAClickThatLeavesThePageTellsTheModelWhereItIsNow(t *testing.T) {
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })

	elsewhere := editorTree()
	elsewhere.App, elsewhere.Window = "pay.example", "Checkout"
	page := &fakeScreen{tree: editorTree(), afterwards: &elsewhere, front: "Google Chrome"}
	steps := &recordingSteps{answers: []synth.StepAnswer{{Step: clickMessage()}, {Done: true, Why: "there"}}}
	e := (&Engine{UIScreen: &browserWindow{}, PageScreen: page, Steps: steps}).onThePage()

	if _, err := e.runDrive(context.Background(), &Plan{Drive: &DrivePlan{App: "Editor", Task: "go to checkout", Web: true, Browser: "Google Chrome"}}); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(steps.requests) < 2 {
		t.Fatalf("asked %d time(s), want a second look at the new page", len(steps.requests))
	}
	told := strings.Join(steps.requests[1].Done, "\n")
	if !strings.Contains(told, "pay.example") || !strings.Contains(told, "Checkout") {
		t.Errorf("the model was not told the browser moved to another page: %q", told)
	}
	if steps.requests[1].Tree.App != "pay.example" {
		t.Errorf("the next screen shown was %q, want the page the click led to", steps.requests[1].Tree.App)
	}
}
