package uiact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	chooserWait = 3 * time.Second
	chooserPoll = 150 * time.Millisecond
	chooserStep = 500 * time.Millisecond
)

var ErrNoChooser = errors.New("uiact: no file chooser opened")

var chooserWindows = map[string]bool{
	"open": true, "open file": true, "open files": true, "choose file": true,
	"choose files": true, "upload": true, "choose": true, "attach": true,
}

var chooserGoes = map[string]bool{
	"open": true, "choose": true, "upload": true, "choose for upload": true, "attach": true, "select": true, "import": true,
}

func HasChooser(t Tree) bool {
	cancel, goes := false, false
	for _, el := range t.Elements {
		switch role(el.Role) {
		case "sheet":
			return true
		case "button":
			name := strings.ToLower(strings.TrimSpace(el.Label()))
			if name == "cancel" {
				cancel = true
			}
			if chooserGoes[name] {
				goes = true
			}
		}
	}
	if cancel && goes {
		return true
	}
	return chooserWindows[strings.ToLower(strings.TrimSpace(t.Window))]
}

var readForChooser = Capture

func waitForChooser(ctx context.Context, r Runner, app string, want bool) bool {
	deadline := time.Now().Add(chooserWait)
	for {
		if tree, err := readForChooser(ctx, r, app); err == nil && HasChooser(tree) == want {
			return true
		}
		if time.Now().After(deadline) || !pause(ctx, chooserPoll) {
			return false
		}
	}
}

const (
	keyG      = 5
	keyEnter  = 36
	keyEscape = 53
)

func ChooseFile(ctx context.Context, r Runner, app, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("uiact: no file to choose")
	}
	if !waitForChooser(ctx, r, app, true) {
		return fmt.Errorf("%w in %s, so nothing was typed", ErrNoChooser, app)
	}

	keyPoster(keyG, Modifiers["Command"]|Modifiers["Shift"])
	if !pause(ctx, chooserStep) {
		return ctx.Err()
	}
	textPoster(path)
	if !pause(ctx, chooserStep) {
		return ctx.Err()
	}
	keyPoster(keyEnter, 0)
	if !pause(ctx, 2*chooserStep) {
		return ctx.Err()
	}
	keyPoster(keyEnter, 0)

	if waitForChooser(ctx, r, app, false) {
		return nil
	}
	keyPoster(keyEscape, 0)
	pause(ctx, chooserStep)
	keyPoster(keyEscape, 0)
	return fmt.Errorf("uiact: the file chooser in %s did not take %s, so it was cancelled", app, path)
}
