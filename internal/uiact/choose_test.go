package uiact

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func uploadTree() Tree {
	return Tree{App: "Site", Elements: []Element{
		{Role: "AXButton", Title: "Select files", Enabled: true, Position: []int{100, 100}, Size: []int{120, 40}, Path: []int{0}},
		{Role: "AXTextField", Title: "Title", Enabled: true, Position: []int{100, 200}, Size: []int{200, 30}, Path: []int{1}},
	}}
}

func TestChoosingAFileNeedsAControlThatOpensAndAPath(t *testing.T) {
	good := Program{App: "Site", Steps: []Step{{Do: "choose", Target: Selector{Role: "AXButton", Title: "Select files"}, Text: "/tmp/clip.mp4"}}}
	if err := Validate(good, uploadTree()); err != nil {
		t.Fatalf("a choose step with a button and a path was refused: %v", err)
	}

	noPath := Program{App: "Site", Steps: []Step{{Do: "choose", Target: Selector{Role: "AXButton", Title: "Select files"}}}}
	if err := Validate(noPath, uploadTree()); err == nil || !strings.Contains(err.Error(), "without giving its path") {
		t.Errorf("err = %v, want the missing path named", err)
	}
}

func TestSwitchingNeedsAnApplicationAndNoTarget(t *testing.T) {
	good := Program{App: "Site", Steps: []Step{{Do: "switch", App: "Notes", Target: Selector{Role: "AXApplication"}}}}
	if err := Validate(good, uploadTree()); err != nil {
		t.Fatalf("a switch step was held to a target it does not have: %v", err)
	}
	if acts, err := Plan(good, uploadTree()); err != nil || len(acts) != 0 {
		t.Errorf("acts = %v, err = %v; switching is not a click or a key in this window", acts, err)
	}

	nowhere := Program{App: "Site", Steps: []Step{{Do: "switch", Target: Selector{Role: "AXApplication"}}}}
	if err := Validate(nowhere, uploadTree()); err == nil {
		t.Error("a switch that names no application was accepted")
	}
}

func TestALetterIsOnlyPressedWithCommand(t *testing.T) {
	field := Selector{Role: "AXTextField", Title: "Title"}
	paste := Program{App: "Site", Steps: []Step{{Do: "press", Key: "V", Modifiers: []string{"Command"}, Target: field}}}
	if err := Validate(paste, uploadTree()); err != nil {
		t.Fatalf("Command-V was refused: %v", err)
	}
	bare := Program{App: "Site", Steps: []Step{{Do: "press", Key: "V", Target: field}}}
	if err := Validate(bare, uploadTree()); err == nil || !strings.Contains(err.Error(), "Command") {
		t.Errorf("err = %v, want a bare letter refused", err)
	}
	down := Program{App: "Site", Steps: []Step{{Do: "press", Key: "Down", Target: field}}}
	if err := Validate(down, uploadTree()); err != nil {
		t.Errorf("an arrow key was refused: %v", err)
	}
}

func TestAChooserIsToldFromAnOrdinaryWindow(t *testing.T) {
	sheet := uploadTree()
	sheet.Elements = append(sheet.Elements, Element{Role: "AXSheet", Path: []int{2}})
	buttons := uploadTree()
	buttons.Elements = append(buttons.Elements,
		Element{Role: "AXButton", Title: "Cancel", Path: []int{2}},
		Element{Role: "AXButton", Title: "Open", Path: []int{3}})
	onlyCancel := uploadTree()
	onlyCancel.Elements = append(onlyCancel.Elements, Element{Role: "AXButton", Title: "Cancel", Path: []int{2}})

	if !HasChooser(sheet) || !HasChooser(buttons) || !HasChooser(Tree{Window: "Open"}) {
		t.Error("a file chooser was not recognised")
	}
	if HasChooser(uploadTree()) || HasChooser(onlyCancel) {
		t.Error("a window with no file chooser was taken for one")
	}
}

type keyLog struct {
	keys  []int
	typed []string
}

func watchTheKeys(t *testing.T, trees ...Tree) *keyLog {
	t.Helper()
	log := &keyLog{}
	key, text, read, wait, poll, step := keyPoster, textPoster, readForChooser, chooserWait, chooserPoll, chooserStep
	keyPoster = func(code int, _ uint64) { log.keys = append(log.keys, code) }
	textPoster = func(s string) { log.typed = append(log.typed, s) }
	chooserWait, chooserPoll, chooserStep = 40*time.Millisecond, time.Millisecond, time.Millisecond
	reads := 0
	readForChooser = func(context.Context, Runner, string) (Tree, error) {
		at := reads
		if at >= len(trees) {
			at = len(trees) - 1
		}
		reads++
		return trees[at], nil
	}
	t.Cleanup(func() {
		keyPoster, textPoster, readForChooser, chooserWait, chooserPoll, chooserStep = key, text, read, wait, poll, step
	})
	return log
}

func TestTheChooserIsWorkedByGoingToThePath(t *testing.T) {
	open := Tree{App: "Site", Window: "Open"}
	log := watchTheKeys(t, open, uploadTree())

	if err := ChooseFile(context.Background(), nil, "Site", "/tmp/clip.mp4"); err != nil {
		t.Fatalf("ChooseFile: %v", err)
	}
	if len(log.typed) != 1 || log.typed[0] != "/tmp/clip.mp4" {
		t.Errorf("typed %q, want the path once", log.typed)
	}
	if len(log.keys) != 3 || log.keys[0] != keyG || log.keys[1] != keyEnter || log.keys[2] != keyEnter {
		t.Errorf("keys = %v, want go-to-folder then Enter twice", log.keys)
	}
}

func TestNothingIsTypedWhenNoChooserOpened(t *testing.T) {
	log := watchTheKeys(t, uploadTree())

	err := ChooseFile(context.Background(), nil, "Site", "/tmp/clip.mp4")
	if !errors.Is(err, ErrNoChooser) {
		t.Fatalf("err = %v, want it to say no chooser opened", err)
	}
	if len(log.keys) != 0 || len(log.typed) != 0 {
		t.Errorf("pressed %v and typed %q into a window that is not a file chooser", log.keys, log.typed)
	}
}

func TestAChooserThatWillNotCloseIsCancelled(t *testing.T) {
	log := watchTheKeys(t, Tree{App: "Site", Window: "Open"})

	if err := ChooseFile(context.Background(), nil, "Site", "/tmp/missing.mp4"); err == nil {
		t.Fatal("a chooser that never took the file was reported as done")
	}
	if n := len(log.keys); n < 2 || log.keys[n-1] != keyEscape {
		t.Errorf("keys = %v, want the chooser cancelled at the end", log.keys)
	}
}
