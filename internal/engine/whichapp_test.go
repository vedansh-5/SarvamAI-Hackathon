package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/synth"
)

type saysApp struct {
	choice synth.AppChoice
	err    error

	asked synth.AppChoiceRequest
	calls int
}

func (s *saysApp) WhichApp(_ context.Context, req synth.AppChoiceRequest) (synth.AppChoice, error) {
	s.calls++
	s.asked = req
	return s.choice, s.err
}

// runningScreen answers what is open, which the plain fakeScreen does not.
type runningScreen struct {
	fakeScreen
	open []string
}

func (r *runningScreen) Running() []string { return r.open }

func chooserEngine(t *testing.T, c *saysApp, open []string) *Engine {
	t.Helper()
	return &Engine{
		Apps:     aMac(t),
		Chooser:  c,
		UIScreen: &runningScreen{fakeScreen: fakeScreen{front: "Antigravity IDE"}, open: open},
		Log:      func(string, ...any) {},
	}
}

// The behaviour this whole thing exists for: asking how to change the wallpaper
// while looking at an editor used to hunt for wallpaper controls in the editor.
func TestAppForTaskLeavesTheWindowInFrontWhenTheTaskIsElsewhere(t *testing.T) {
	chooser := &saysApp{choice: synth.AppChoice{App: "System Settings", Why: "appearance lives there"}}
	e := chooserEngine(t, chooser, []string{"System Settings", "Antigravity IDE"})

	got, err := e.appForTask(context.Background(), "how do I change the wallpaper",
		&screen.Context{AppName: "Antigravity IDE"})
	if err != nil {
		t.Fatalf("appForTask: %v", err)
	}
	if got != "System Settings" {
		t.Errorf("app = %q, want System Settings", got)
	}
	if chooser.asked.InFront != "Antigravity IDE" {
		t.Errorf("the model was not told what was in front: %+v", chooser.asked)
	}
	if len(chooser.asked.Installed) == 0 {
		t.Error("the model was given no installed applications to choose from")
	}
}

// An application that is open but behind something is fine — the coach raises
// its own target. One that is not running at all is the person's to open.
func TestAppForTaskSaysWhenTheAppIsNotOpen(t *testing.T) {
	chooser := &saysApp{choice: synth.AppChoice{App: "System Settings", Why: "appearance lives there"}}
	e := chooserEngine(t, chooser, []string{"Antigravity IDE"})

	_, err := e.appForTask(context.Background(), "how do I change the wallpaper",
		&screen.Context{AppName: "Antigravity IDE"})

	var notOpen *AppNotOpenError
	if !errors.As(err, &notOpen) {
		t.Fatalf("err = %v, want an AppNotOpenError the panel can act on", err)
	}
	if notOpen.App != "System Settings" {
		t.Errorf("App = %q, want System Settings", notOpen.App)
	}
	if notOpen.Why == "" {
		t.Error("nothing to tell the person about why they are being sent there")
	}
}

// Being asked the question is not a reason to send someone somewhere else.
func TestAppForTaskStaysPutWhenTheTaskBelongsThere(t *testing.T) {
	chooser := &saysApp{choice: synth.AppChoice{App: "Telegram"}}
	e := chooserEngine(t, chooser, []string{"Telegram"})

	got, err := e.appForTask(context.Background(), "how do I start a new chat",
		&screen.Context{AppName: "Telegram"})
	if err != nil {
		t.Fatalf("appForTask: %v", err)
	}
	if got != "Telegram" {
		t.Errorf("app = %q, want Telegram", got)
	}
}

// Nothing installed does it, which is a real answer.
func TestAppForTaskReportsNowhere(t *testing.T) {
	chooser := &saysApp{choice: synth.AppChoice{Nowhere: true, Why: "no torrent client is installed"}}
	e := chooserEngine(t, chooser, nil)

	if _, err := e.appForTask(context.Background(), "how do I seed a torrent", nil); !errors.Is(err, ErrNowhereToDoIt) {
		t.Errorf("err = %v, want ErrNowhereToDoIt", err)
	}
}

// Failing to improve on the old behaviour is not a reason to refuse the request.
func TestAppForTaskFallsBackToTheWindowInFront(t *testing.T) {
	chooser := &saysApp{err: errors.New("synth: the model did not answer")}
	e := chooserEngine(t, chooser, []string{"Antigravity IDE"})

	got, err := e.appForTask(context.Background(), "how do I change the wallpaper",
		&screen.Context{AppName: "Antigravity IDE"})
	if err != nil {
		t.Fatalf("appForTask: %v", err)
	}
	if got != "Antigravity IDE" {
		t.Errorf("app = %q, want the window in front when the choice cannot be made", got)
	}
}

// A name the model invented cannot be opened or read, so it is not an answer.
func TestAppForTaskIgnoresAnAppThatIsNotInstalled(t *testing.T) {
	chooser := &saysApp{choice: synth.AppChoice{App: "Wallpaper Changer 9000"}}
	e := chooserEngine(t, chooser, []string{"Antigravity IDE"})

	got, err := e.appForTask(context.Background(), "how do I change the wallpaper",
		&screen.Context{AppName: "Antigravity IDE"})
	if err != nil {
		t.Fatalf("appForTask: %v", err)
	}
	if got != "Antigravity IDE" {
		t.Errorf("app = %q, want the window in front", got)
	}
}

func TestTheApplicationInFrontCanBeChosenEvenIfItIsNotInstalledHere(t *testing.T) {
	chooser := &saysApp{choice: synth.AppChoice{App: "Ledger Dev Build"}}
	e := chooserEngine(t, chooser, []string{"Ledger Dev Build"})
	e.UIScreen = &runningScreen{fakeScreen: fakeScreen{front: "Ledger Dev Build"}, open: []string{"Ledger Dev Build"}}

	got, err := e.appForTask(context.Background(), "how do I add an entry",
		&screen.Context{AppName: "Ledger Dev Build"})
	if err != nil {
		t.Fatalf("appForTask: %v", err)
	}
	if got != "Ledger Dev Build" {
		t.Errorf("app = %q, want the one in front", got)
	}
	if !named(chooser.asked.Installed, "Ledger Dev Build") {
		t.Errorf("the model was not offered the application in front: %v", chooser.asked.Installed)
	}
}

type keptPlaces map[string]string

func (k keptPlaces) Setting(_ context.Context, key string) (string, error) { return k[key], nil }
func (k keptPlaces) SetSetting(_ context.Context, key, value string) error {
	k[key] = value
	return nil
}

func TestAGuideShownBeforeDoesNotAskAgainWhereItsTaskLives(t *testing.T) {
	const shape = "how do i change the wallpaper"
	chooser := &saysApp{choice: synth.AppChoice{App: "System Settings", Why: "appearance lives there"}}
	e := chooserEngine(t, chooser, []string{"System Settings", "Antigravity IDE"})
	e.Places = keptPlaces{}
	e.Steps = &alwaysStep{step: typeIntoMessage()}

	e.rememberThePlace(context.Background(), &GuidePlan{App: "System Settings", Shape: shape, Elsewhere: true, Why: "appearance lives there"})

	app, err, known := e.placeRemembered(context.Background(), shape, "how do I change the wallpaper")
	if !known || err != nil || app != "System Settings" {
		t.Fatalf("app = %q, err = %v, known = %v, want System Settings remembered", app, err, known)
	}
}

func TestARememberedPlaceThatStopsWorkingIsForgottenAndAskedAbout(t *testing.T) {
	const shape = "how do i change the wallpaper"
	chooser := &saysApp{choice: synth.AppChoice{App: "System Settings"}}
	e := chooserEngine(t, chooser, []string{"System Settings", "Antigravity IDE"})
	places := keptPlaces{placeKey(shape): "System Settings\t"}
	e.Places = places
	e.Steps = &alwaysStep{step: typeIntoMessage()}

	e.planGuideFor(context.Background(), "how do I change the wallpaper", shape, nil, &screen.Context{AppName: "Antigravity IDE"})

	if places[placeKey(shape)] != "" {
		t.Errorf("kept %q although the window it led to had nothing to point at", places[placeKey(shape)])
	}
	if chooser.calls != 1 {
		t.Errorf("the model was asked %d time(s), want once after forgetting", chooser.calls)
	}
}

func TestARememberedPlaceThatIsNotOpenIsStillAStepToOpenIt(t *testing.T) {
	const shape = "how do i change the wallpaper"
	e := chooserEngine(t, &saysApp{}, []string{"Antigravity IDE"})
	e.Places = keptPlaces{placeKey(shape): "System Settings\tappearance lives there"}

	_, err, known := e.placeRemembered(context.Background(), shape, "how do I change the wallpaper")
	var notOpen *AppNotOpenError
	if !known || !errors.As(err, &notOpen) || notOpen.App != "System Settings" || notOpen.Why != "appearance lives there" {
		t.Fatalf("err = %v, known = %v, want System Settings reported as not open, with why", err, known)
	}
}

func TestATaskDoneInTheApplicationInFrontIsNotRemembered(t *testing.T) {
	places := keptPlaces{}
	e := &Engine{Places: places}

	e.rememberThePlace(context.Background(), &GuidePlan{App: "Notes", Shape: "where is the search box", Elsewhere: false})
	e.rememberThePlace(context.Background(), &GuidePlan{App: "example.com", Shape: "how do i log in", Elsewhere: true, Web: true})

	if len(places) != 0 {
		t.Errorf("remembered %v; in front depends on where they were, and a page is not an application", places)
	}
}
