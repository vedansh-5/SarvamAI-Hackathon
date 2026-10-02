package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/uiact"
)

type lookingKey struct{}

const longestSelection = 600

var pointsAtWhatIsInFront = regexp.MustCompile(`(?i)\b(this|that|these|those|here|it|selected|selection|highlighted|current|above|the one i('| a)?m (looking at|on|in)|what i('| a)?m (looking at|reading)|on (my |the )?screen|in front of me|open (page|tab|window|document|file|chat|email|message))\b`)

func refersToWhatIsInFront(instruction string) bool {
	return pointsAtWhatIsInFront.MatchString(instruction)
}

func whatTheyWereLookingAt(at *screen.Context, selection string) string {
	if at == nil {
		return ""
	}
	app := strings.TrimSpace(at.AppName)
	if app == "" || strings.EqualFold(app, "vans") {
		return ""
	}
	window := strings.TrimSpace(at.WindowTitle)
	if len([]rune(window)) > 120 {
		window = string([]rune(window)[:120])
	}

	note := fmt.Sprintf("When they asked, they were looking at %s", app)
	if window != "" && !strings.EqualFold(window, app) {
		note += fmt.Sprintf(", in the window %q", window)
	}
	note += "."

	selection = strings.TrimSpace(selection)
	if selection != "" {
		if len([]rune(selection)) > longestSelection {
			selection = string([]rune(selection)[:longestSelection]) + "\u2026"
		}
		note += fmt.Sprintf(" They had this text selected: %q.", selection)
		return note + " Words like \"this\", \"that\" and \"it\" in the task mean that text."
	}
	return note + " Words like \"this\", \"here\" and \"it\" in the task mean what was in front of them there."
}

func (e *Engine) selectionIn(ctx context.Context, at *screen.Context) string {
	if at == nil || strings.TrimSpace(at.AppName) == "" {
		return ""
	}
	app := strings.TrimSpace(at.AppName)
	if e.isPageBrowser(ctx, app) {
		if reader, ok := e.PageScreen.(uiact.SelectionReader); ok {
			if picked, ok := reader.Selection(ctx, app); ok {
				return picked
			}
		}
		return ""
	}
	if reader, ok := e.UIScreen.(uiact.SelectionReader); ok {
		if picked, ok := reader.Selection(ctx, app); ok {
			return picked
		}
	}
	return ""
}

func (e *Engine) lookingFor(ctx context.Context, in Ask) string {
	if !refersToWhatIsInFront(in.Instruction) {
		return ""
	}
	if files := e.filesSelectedIn(ctx, in.Context); len(files) > 0 {
		return filesNote(files)
	}
	return whatTheyWereLookingAt(in.Context, e.selectionIn(ctx, in.Context))
}

func withLooking(ctx context.Context, note string) context.Context {
	if note == "" {
		return ctx
	}
	return context.WithValue(ctx, lookingKey{}, note)
}

func taskWithLooking(ctx context.Context, task string) string {
	note, _ := ctx.Value(lookingKey{}).(string)
	if note == "" {
		return task
	}
	return task + "\n\n" + note
}
