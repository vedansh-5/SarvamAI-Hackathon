package engine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/uiact"
)

// ErrCoachStopped marks a coached run that ended because of what was on screen,
// rather than because something in VANS failed.
//
// Every one of these messages is accurate and was written to be read by a person
// — "no part of the AXButton \"Tahoe\" is visible on screen" says exactly what
// happened. They were all arriving at the panel under "Something went wrong
// inside VANS", which is the one thing they do not mean: nothing went wrong
// inside VANS, the window was not what the step needed. The heading sent people
// looking for a fault on the wrong side.
var ErrCoachStopped = errors.New("engine: the steps stopped")

// stoppedError carries the sentinel without disturbing the sentence. Wrapping
// with %w would append the sentinel's own text to messages that are already
// finished.
type stoppedError struct {
	app string
	msg string
}

func (e *stoppedError) Error() string { return e.msg }
func (e *stoppedError) Unwrap() error { return ErrCoachStopped }

// App is which application the run was working in, for the panel to name.
func (e *stoppedError) App() string { return e.app }

func stopped(app, format string, args ...any) error {
	return &stoppedError{app: app, msg: fmt.Sprintf(format, args...)}
}

// WhereItStopped reports the application a stopped run was working in.
func WhereItStopped(err error) string {
	var s *stoppedError
	if errors.As(err, &s) {
		return s.App()
	}
	return ""
}

var plainKinds = map[string]string{
	"AXButton": "button", "AXTextField": "box", "AXTextArea": "box", "AXSearchField": "search box",
	"AXComboBox": "box", "AXSecureTextField": "box", "AXCheckBox": "checkbox", "AXRadioButton": "option",
	"AXPopUpButton": "menu", "AXMenuButton": "menu", "AXMenuItem": "menu item", "AXLink": "link",
	"AXTab": "tab", "AXSlider": "slider", "AXRow": "row", "AXCell": "cell", "AXImage": "picture",
}

func called(sel uiact.Selector) string {
	if title := strings.TrimSpace(sel.Title); title != "" {
		return "“" + title + "”"
	}
	if kind, ok := plainKinds[sel.Role]; ok {
		return "the " + kind
	}
	return "that control"
}

func oneOfSeveral(sel uiact.Selector) string {
	if title := strings.TrimSpace(sel.Title); title != "" {
		return "“" + title + "”"
	}
	if kind, ok := plainKinds[sel.Role]; ok {
		return kind
	}
	return "control like it"
}

func cannotPointAt(app string, sel uiact.Selector, why string) error {
	name := called(sel)
	switch {
	case strings.Contains(why, "visible"):
		return stopped(app, "engine: Vspk cannot see %s: it is covered or off screen. Move the window so it shows, then ask again.", name)
	case strings.Contains(why, "does not say which"), strings.Contains(why, "number"):
		return stopped(app, "engine: There is more than one %s here, and Vspk could not tell which one is meant. Ask again and say where it is.", oneOfSeveral(sel))
	}
	return stopped(app, "engine: Vspk cannot see %s any more. Ask again to start from what the window shows now.", name)
}
