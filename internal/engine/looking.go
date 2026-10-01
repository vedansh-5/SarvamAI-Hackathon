package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/screen"
)

type lookingKey struct{}

func whatTheyWereLookingAt(at *screen.Context) string {
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
	if window == "" || strings.EqualFold(window, app) {
		return fmt.Sprintf("When they asked, they were looking at %s. Words like \"this\", \"here\" and \"it\" in the task mean what was in front of them there.", app)
	}
	return fmt.Sprintf("When they asked, they were looking at %s, in the window %q. Words like \"this\", \"here\" and \"it\" in the task mean what was in front of them there.", app, window)
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
