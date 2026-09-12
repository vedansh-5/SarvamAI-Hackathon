package engine

import (
	"strings"

	"github.com/vedansh/vans/internal/route"
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

func (e *Engine) Suggestions(front string) []Suggestion {
	candidates := []Suggestion{
		{Icon: "page", Text: "what is this page about", Kind: route.KindPageHere},
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

	if browsers[strings.ToLower(strings.TrimSpace(front))] {
		out = pageFirst(out)
	}
	return out
}

func (e *Engine) offers(s Suggestion) bool {
	if !e.runs(s.Kind) {
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
