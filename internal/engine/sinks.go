package engine

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/uiact"
	"sort"
	"strings"
)

// Where an answer goes is a choice, not a constant.
//
// V1 wrote to Notes because Notes was the only destination that existed, and
// the confirmation gate was therefore an unconditional step. Neither holds now:
// most of what a person asks for they want to *read*, and reading changes
// nothing in the world.
//
// So the destination decides whether the gate appears. That is not a
// convenience — it is D-004's taxonomy applied honestly. A gate in front of an
// action with no side effect trains people to dismiss gates, which is exactly
// what must not happen in front of the ones that matter.

// PanelDestination shows the answer in the panel and writes nothing anywhere.
//
// It is the default, and it has no Sink behind it: the plan already carries
// the rows, so there is nothing left to do. Being a name rather than an
// implementation is the point — there is no code path from "show me" to a
// side effect.
const PanelDestination = "panel"

// Destination describes somewhere an answer can go.
type Destination struct {
	Name     string
	Describe string

	// TouchesWorld reports whether choosing this destination means something
	// outside VANS changes. It is what the gate keys on.
	TouchesWorld bool
}

// Sink is a destination that can be written to.
type Sink interface {
	// Write commits the document and returns a receipt. An implementation
	// that can read its own writes back must do so; the receipt says whether
	// it did.
	Write(ctx context.Context, doc Document) (Receipt, error)

	// Destination describes this sink to the user and to the gate.
	Destination() Destination
}

// Destinations lists everywhere this engine can send an answer, panel first.
func (e *Engine) Destinations() []Destination {
	out := []Destination{{
		Name:         PanelDestination,
		Describe:     "Show it here. Nothing is written anywhere.",
		TouchesWorld: false,
	}}
	for _, s := range e.Sinks {
		out = append(out, s.Destination())
	}
	sort.Slice(out[1:], func(i, j int) bool {
		return out[1:][i].Name < out[1:][j].Name
	})
	return out
}

// sinkFor resolves a destination name to a sink.
func (e *Engine) sinkFor(name string) (Sink, error) {
	if name == "" || name == PanelDestination {
		return nil, fmt.Errorf("engine: %q writes nothing, so there is nothing to commit", PanelDestination)
	}
	if s, ok := e.Sinks[name]; ok {
		return s, nil
	}
	var known []string
	for _, d := range e.Destinations() {
		known = append(known, d.Name)
	}
	return nil, fmt.Errorf("engine: no destination named %q (have %s)", name, strings.Join(known, ", "))
}

// ClipboardSink puts the answer on the pasteboard.
//
// A side effect, and so gated — it destroys whatever the user had copied,
// which is not recoverable and not something to do on their behalf without
// asking. It is nonetheless the cheapest way to get an answer into an
// application VANS cannot otherwise reach.
type ClipboardSink struct{}

func (ClipboardSink) Destination() Destination {
	return Destination{
		Name:         "clipboard",
		Describe:     "Copy it, replacing whatever you had copied.",
		TouchesWorld: true,
	}
}

func (ClipboardSink) Write(ctx context.Context, doc Document) (Receipt, error) {
	text := doc.Text()

	cmd := exec.CommandContext(ctx, "pbcopy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return Receipt{}, fmt.Errorf("engine: copy to clipboard: %w", err)
	}

	// Read back, because it can be. D-013: a tier that can confirm its own
	// write must, and the receipt reports whether it did rather than assuming.
	out, err := exec.CommandContext(ctx, "pbpaste").Output()
	if err != nil {
		return Receipt{Title: doc.Title}, nil
	}
	return Receipt{
		Title:    doc.Title,
		Verified: string(out) == text,
	}, nil
}

// Text renders a document as tab-separated lines.
//
// Tabs rather than commas: the destination is a pasteboard, and every
// spreadsheet and note-taker on the machine reads tab-separated text as
// columns without being told to.
func (d Document) Text() string {
	var b strings.Builder
	b.WriteString(strings.Join(d.Fields, "\t"))
	b.WriteByte('\n')
	for _, row := range d.Rows {
		cells := make([]string, len(d.Fields))
		for i, f := range d.Fields {
			// Newlines and tabs inside a cell would silently become new rows
			// and new columns in whatever reads this.
			cells[i] = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(row[f])
		}
		b.WriteString(strings.Join(cells, "\t"))
		b.WriteByte('\n')
	}
	return b.String()
}

// DefaultSinks is the set every VANS build ships with.
//
// Assembled here rather than in each binary so the CLI and the desktop shell
// cannot drift into offering different destinations — a difference the user
// would experience as the same request behaving differently depending on how
// they started it.
func DefaultSinks(extra ...Sink) map[string]Sink {
	out := map[string]Sink{}
	base := []Sink{ClipboardSink{}, AppSink{Screen: uiact.System{Runner: appleevents.Osascript{}}}}
	for _, s := range append(base, extra...) {
		out[s.Destination().Name] = s
	}
	return out
}
