package engine

import (
	"context"

	"github.com/vedansh/vans/internal/gapps"
)

type FrontAsker interface {
	AskFront(ctx context.Context, script string, out any) error
}

func (e *Engine) googleFileIn(ctx context.Context, reader FrontReader, pageURL, src string) string {
	f, ok := gapps.Recognise(pageURL)
	if !ok {
		return src
	}
	asker, ok := reader.(FrontAsker)
	if !ok {
		return src
	}
	script, err := f.ReadScript()
	if err != nil {
		return src
	}
	var got gapps.Export
	if err := asker.AskFront(ctx, script, &got); err != nil {
		e.logf("could not ask the %s for its text: %v", f.Kind, err)
		return src
	}
	if err := got.Err(); err != nil {
		e.logf("reading the %s as drawn: %v", f.Kind, err)
		return src
	}
	page, err := gapps.Page(f, gapps.Title(src), got.Text)
	if err != nil {
		e.logf("reading the %s as drawn: %v", f.Kind, err)
		return src
	}
	e.logf("read the %s from its own export, %d bytes%s", f.Kind, len(got.Text), map[bool]string{true: ", cut short"}[got.Cut])
	return page
}
