// Package runner executes an extraction program against a DOM and checks the
// result against its contract.
//
// This is the deterministic replay half of the runtime. Synthesis writes a
// program once; this package runs it on every subsequent visit with no model
// involved. That is the whole cost argument, so nothing here may reach for a
// network, a model, or a heuristic that could quietly change its mind between
// two runs of the same program on the same page.
//
// Running against a parsed HTML string rather than querying the live page over
// CDP is deliberate. The page signature, the candidate regions, and the program
// run all have to describe the same DOM. Querying the browser separately would
// evaluate selectors against a snapshot taken later than the one that produced
// the signature, so a page that mutated in between would pass verification
// while returning data from a shape the cache key does not describe.
package runner

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"

	"github.com/vedansh/vans/internal/extract"
)

// Row is one extracted record, keyed by field name.
type Row map[string]string

// FieldStat records how a field behaved across the whole run. It exists to
// make failure reasons specific: "the selector matched nothing" and "the
// selector matched an element with no text" are different defects and the
// model needs to be told which one it wrote.
type FieldStat struct {
	// Matched counts rows where the field's selector matched an element.
	Matched int

	// NonEmpty counts rows where the extracted value was non-empty.
	NonEmpty int
}

// Result is what a program produced.
type Result struct {
	// Fields is the field order declared by the program.
	Fields []string

	// Rows are the extracted records, empty rows already removed.
	Rows []Row

	// Matched is how many elements the root selector matched, before empty
	// rows were dropped.
	Matched int

	// Dropped is how many matched records yielded no values at all.
	Dropped int

	// Stats is per-field extraction behaviour, keyed by field name.
	Stats map[string]FieldStat
}

// Options tunes a run.
type Options struct {
	// Base resolves relative href and src values. Without it a program that
	// extracts links returns "/p/1", which is useless once it leaves the
	// browser and lands in a note.
	Base *url.URL

	// MaxRows caps how many elements the root selector may match. Defaults to
	// DefaultMaxRows. Exceeding it is a program defect, not a big page: a root
	// broad enough to match thousands of elements is matching the page's
	// layout rather than its records.
	MaxRows int
}

// DefaultMaxRows bounds a runaway root selector.
const DefaultMaxRows = 1000

// urlAttrs are resolved against Options.Base. The other allowed attributes are
// plain values and must be passed through untouched.
var urlAttrs = map[string]bool{"href": true, "src": true}

// ProgramError means the program cannot be executed at all — a selector that
// does not parse, or a root that matches far too much.
//
// It is distinct from a verification failure on purpose. A verification failure
// might just mean the page changed, so the ladder first invalidates the cache.
// A ProgramError means what was cached was never executable, and re-running it
// against a fresher DOM would fail identically.
type ProgramError struct {
	Reason string
}

func (e *ProgramError) Error() string { return "runner: " + e.Reason }

// Failures renders the error as re-synthesis input.
func (e *ProgramError) Failures() []string { return []string{e.Reason} }

// Run executes p against doc.
//
// It reports a ProgramError for a program it cannot execute, and otherwise
// returns whatever the program produced — including a result that will fail
// verification. Judging the output is Verify's job, and keeping the two apart
// is what lets the failure reasons describe the actual data.
func Run(doc *html.Node, p extract.Program, opts Options) (Result, error) {
	if err := p.Validate(); err != nil {
		return Result{}, &ProgramError{Reason: err.Error()}
	}
	if opts.MaxRows == 0 {
		opts.MaxRows = DefaultMaxRows
	}

	root, err := cascadia.Compile(p.Root)
	if err != nil {
		return Result{}, &ProgramError{
			Reason: fmt.Sprintf("root selector %q does not parse: %v", p.Root, err),
		}
	}

	// Field selectors are compiled up front so a syntax error is reported
	// before any extraction happens, rather than surfacing as a field that
	// silently produced nothing.
	sels := make([]cascadia.Selector, len(p.Fields))
	for i, f := range p.Fields {
		if strings.TrimSpace(f.Selector) == "" {
			continue // the record element itself
		}
		s, err := cascadia.Compile(f.Selector)
		if err != nil {
			return Result{}, &ProgramError{
				Reason: fmt.Sprintf("field %q selector %q does not parse: %v", f.Name, f.Selector, err),
			}
		}
		sels[i] = s
	}

	records := cascadia.QueryAll(doc, root)

	res := Result{
		Fields:  p.FieldNames(),
		Matched: len(records),
		Stats:   make(map[string]FieldStat, len(p.Fields)),
	}

	if len(records) > opts.MaxRows {
		return res, &ProgramError{
			Reason: fmt.Sprintf("root selector %q matched %d elements, above the limit of %d; it is selecting layout rather than records",
				p.Root, len(records), opts.MaxRows),
		}
	}

	for _, rec := range records {
		row := make(Row, len(p.Fields))
		any := false

		for i, f := range p.Fields {
			node := rec
			if sels[i] != nil {
				node = cascadia.Query(rec, sels[i])
			}

			st := res.Stats[f.Name]
			if node == nil {
				res.Stats[f.Name] = st
				row[f.Name] = ""
				continue
			}
			st.Matched++

			v := value(node, f.Attr, opts.Base)
			if v != "" {
				st.NonEmpty++
				any = true
			}
			res.Stats[f.Name] = st
			row[f.Name] = v
		}

		// A record with nothing in any field carries no data by definition —
		// keeping it could only put a blank line in the output. Dropping is
		// recorded rather than hidden, because a high drop rate is itself
		// evidence the root selector is wrong. Verify decides that.
		if !any {
			res.Dropped++
			continue
		}
		res.Rows = append(res.Rows, row)
	}

	return res, nil
}

// value reads one field off a matched element.
func value(n *html.Node, attrName string, base *url.URL) string {
	if attrName == "text" {
		return extract.Text(n)
	}

	var v string
	for _, a := range n.Attr {
		if a.Key == attrName {
			v = strings.TrimSpace(a.Val)
			break
		}
	}
	if v == "" || base == nil || !urlAttrs[attrName] {
		return v
	}

	// A URL that will not parse is passed through as written. Blanking it
	// would turn a cosmetic problem into a missing field, and a missing field
	// is what sends the program back for re-synthesis.
	ref, err := url.Parse(v)
	if err != nil {
		return v
	}
	return base.ResolveReference(ref).String()
}

// RunHTML parses src and runs p against it.
func RunHTML(src string, p extract.Program, opts Options) (Result, error) {
	doc, err := extract.Parse(src)
	if err != nil {
		return Result{}, fmt.Errorf("runner: parse page: %w", err)
	}
	return Run(doc, p, opts)
}
