package engine

import (
	"context"
	"strings"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/store"
)

type Suggestion struct {
	Icon        string
	Text        string
	Destination string

	Kind route.Kind
}

var browsers = map[string]bool{
	"google chrome":  true,
	"safari":         true,
	"arc":            true,
	"firefox":        true,
	"microsoft edge": true,
	"brave browser":  true,
}

// isBrowser is asked in three places and the answer means something different in
// each: which suggestions to put first, which tier reads the window, and whether
// the coach can point at anything at all.
func isBrowser(app string) bool {
	return browsers[strings.ToLower(strings.TrimSpace(app))]
}

type History interface {
	RecordInstruction(ctx context.Context, text, kind string) error
	RecentInstructions(ctx context.Context) ([]store.Remembered, error)
}

func (e *Engine) Suggestions(ctx context.Context, front string) []Suggestion {
	candidates := []Suggestion{
		{Icon: "page", Text: "what is this page about", Kind: route.KindPageHere},
		{Icon: "pencil", Text: "translate this to Hindi", Kind: route.KindText},
		{Icon: "page", Text: "move the screenshots on my Desktop into a folder called Screenshots", Kind: route.KindFiles},
		{Icon: "pencil", Text: "switch to dark mode", Kind: route.KindSetting},
		{Icon: "search", Text: "search for noise-cancelling headphones under $200", Kind: route.KindWebSearch},
		{Icon: "chart", Text: "search for today's biggest NASDAQ movers", Kind: route.KindWebSearch},
		{Icon: "rss", Text: "the top stories on news.ycombinator.com", Kind: route.KindWebExtract},
		{Icon: "pencil", Text: "the top stories on news.ycombinator.com", Destination: "notes", Kind: route.KindWebExtract},
	}

	var out []Suggestion
	for _, s := range candidates {
		if e.offers(s) {
			out = append(out, s)
		}
	}

	if s, ok := e.appExample(front); ok {
		out = append(out, s)
	}

	if e.canGuide() {
		out = append(out, Suggestion{
			Icon: "page",
			Text: "show me where the search box is",
			Kind: route.KindAppGuide,
		})
	}

	if e.canDrivePages() {
		out = append(out, Suggestion{
			Icon: "page",
			Text: "search for flights from Delhi to Goa on makemytrip",
			Kind: route.KindPageAction,
		})
	}

	if e.canStyle() {
		out = append(out,
			Suggestion{Icon: "pencil", Text: "highlight the headings on this page", Kind: route.KindPageStyle},
			Suggestion{Icon: "page", Text: "clear the highlighting from this page", Kind: route.KindPageStyle})
	}

	if isBrowser(front) {
		out = pageFirst(out)
	}
	return append(e.remembered(ctx, out), out...)
}

var appExamples = []Suggestion{
	{Icon: "pencil", Text: "make a note titled Groceries in Notes", Kind: route.KindAppAction},
	{Icon: "pencil", Text: "add a reminder to call the dentist tomorrow in Reminders", Kind: route.KindAppAction},
}

func (e *Engine) appExample(front string) (Suggestion, bool) {
	if !e.canRun(route.KindAppAction) || e.Apps == nil {
		return Suggestion{}, false
	}

	var installed []string
	for _, a := range e.Apps.Scriptable() {
		installed = append(installed, a.Name)
	}
	return appExampleFor(installed, front)
}

func appOf(s Suggestion) string {
	fields := strings.Fields(s.Text)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(fields[len(fields)-1])
}

func appExampleFor(installed []string, front string) (Suggestion, bool) {
	have := map[string]bool{}
	for _, name := range installed {
		have[strings.ToLower(strings.TrimSpace(name))] = true
	}

	if f := strings.ToLower(strings.TrimSpace(front)); f != "" && have[f] {
		for _, s := range appExamples {
			if appOf(s) == f {
				return s, true
			}
		}
	}

	for _, s := range appExamples {
		if have[appOf(s)] {
			return s, true
		}
	}
	return Suggestion{}, false
}

func (e *Engine) remembered(ctx context.Context, already []Suggestion) []Suggestion {
	if e.History == nil {
		return nil
	}

	recent, err := e.History.RecentInstructions(ctx)
	if err != nil {
		e.logf("could not read what was asked before: %v", err)
		return nil
	}

	seen := map[string]bool{}
	for _, s := range already {
		seen[strings.ToLower(strings.TrimSpace(s.Text))] = true
	}

	var out []Suggestion
	for _, r := range recent {
		key := strings.ToLower(strings.TrimSpace(r.Text))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true

		s := Suggestion{Icon: "search", Text: r.Text, Kind: route.Kind(r.Kind)}
		if !e.offers(s) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (e *Engine) offers(s Suggestion) bool {
	if !e.canRun(s.Kind) {
		return false
	}
	if s.Destination == "" {
		return true
	}
	_, err := e.sinkFor(s.Destination)
	return err == nil
}

func pageFirst(in []Suggestion) []Suggestion {
	out := make([]Suggestion, 0, len(in))
	for _, s := range in {
		if s.Kind == route.KindPageHere {
			out = append(out, s)
		}
	}
	for _, s := range in {
		if s.Kind != route.KindPageHere {
			out = append(out, s)
		}
	}
	return out
}
