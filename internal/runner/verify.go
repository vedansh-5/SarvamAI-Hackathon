package runner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/extract"
)

// MaxDropRate is the share of matched records that may yield nothing before
// the root selector is judged too broad.
//
// Some slack is necessary: real listings interleave placeholder cards, ad
// slots, and skeleton loaders that share the records' shape. Beyond a third,
// the selector is matching a container class rather than the records.
const MaxDropRate = 0.34

// ConfidentRows is how many complete records make a high drop rate acceptable.
//
// The floor above exists to catch a selector that grabbed a container instead
// of the records, and that mistake yields a handful of rows at most. A search
// page is the other case: results, image strips, "people also ask" and video
// carousels are all siblings under one class, so more than half of them
// legitimately hold nothing a record needs — and the rows that do survive are
// the answer. Found on a Google results page that produced eight complete
// records out of nineteen elements and had them thrown away.
const ConfidentRows = 5

// VerifyError is a result that ran but does not satisfy its contract.
//
// It carries every reason rather than the first, because its whole purpose is
// to be handed back to the synthesizer. A model told only "price was empty"
// will fix price and leave the second broken selector for the next round trip,
// and every round trip is a model call the runtime exists to avoid.
type VerifyError struct {
	Reasons []string
}

func (e *VerifyError) Error() string {
	return "runner: result failed verification: " + strings.Join(e.Reasons, "; ")
}

// Failures renders the error as re-synthesis input.
func (e *VerifyError) Failures() []string { return e.Reasons }

// Verify checks a result against the program's contract and the runtime's own
// floor.
//
// The contract half is the model's: it wrote expect, and it is held to it. The
// floor half is the runtime's, and it exists because the model also gets to
// choose how weak its own contract is. A program declaring min_rows 1 and no
// required fields passes any contract check while returning a single row of
// empty strings, so the checks the model cannot weaken are the ones that
// actually protect the user's data.
func Verify(p extract.Program, res Result) error {
	var reasons []string

	minRows := p.Expect.MinRows
	if minRows < 1 {
		minRows = 1
	}

	if len(res.Rows) < minRows {
		switch {
		case res.Matched == 0:
			reasons = append(reasons, fmt.Sprintf(
				"root selector %q matched no elements on the page", p.Root))
		case res.Dropped == res.Matched:
			reasons = append(reasons, fmt.Sprintf(
				"root selector %q matched %d elements but every field was empty in all of them, so the selector is matching the wrong elements",
				p.Root, res.Matched))
		default:
			reasons = append(reasons, fmt.Sprintf(
				"got %d rows, expected at least %d", len(res.Rows), minRows))
		}

		// With no usable rows, every per-field check below would report the
		// same root problem again in field-shaped language.
		if len(res.Rows) == 0 {
			return &VerifyError{Reasons: reasons}
		}
	}

	// The model's own contract: fields it marked required must be present
	// everywhere.
	for _, name := range p.Expect.Required {
		missing := 0
		for _, row := range res.Rows {
			if strings.TrimSpace(row[name]) == "" {
				missing++
			}
		}
		if missing > 0 {
			reasons = append(reasons, fmt.Sprintf(
				"field %q is marked required but is empty in %d of %d rows",
				name, missing, len(res.Rows)))
		}
	}

	// The runtime's floor: a field that is empty in every row is a broken
	// selector, whether or not the model called it required. The user asked
	// for that field by name, so a column of empty strings is a wrong answer
	// being reported as a successful one.
	for _, f := range p.Fields {
		st := res.Stats[f.Name]
		if st.NonEmpty > 0 {
			continue
		}
		if isRequired(p, f.Name) {
			continue // already reported above, in the model's own terms
		}

		sel := f.Selector
		if strings.TrimSpace(sel) == "" {
			sel = "the record element itself"
		} else {
			sel = fmt.Sprintf("%q", sel)
		}

		if st.Matched == 0 {
			reasons = append(reasons, fmt.Sprintf(
				"field %q: selector %s matched nothing in any of the %d records",
				f.Name, sel, len(res.Rows)))
		} else {
			reasons = append(reasons, fmt.Sprintf(
				"field %q: selector %s matched an element in %d records but %s was empty in all of them",
				f.Name, sel, st.Matched, attrLabel(f.Attr)))
		}
	}

	// A root that matches far more elements than it yields rows is selecting
	// something broader than the records — unless what survives is itself a
	// full set of records, which is what a mixed results page looks like.
	tooBroad := res.Matched > 0 && float64(res.Dropped)/float64(res.Matched) > MaxDropRate
	if tooBroad && len(res.Rows) < ConfidentRows {
		reasons = append(reasons, fmt.Sprintf(
			"root selector %q matched %d elements but %d of them held no data; it is broader than the records",
			p.Root, res.Matched, res.Dropped))
	}

	if len(reasons) == 0 {
		return nil
	}
	return &VerifyError{Reasons: reasons}
}

func isRequired(p extract.Program, name string) bool {
	for _, r := range p.Expect.Required {
		if r == name {
			return true
		}
	}
	return false
}

func attrLabel(attr string) string {
	if attr == "text" {
		return "its text"
	}
	return "the " + attr + " attribute"
}

// Execute runs a program and verifies it in one step.
//
// It returns the result alongside any error, because a failed result is the
// input to re-synthesis and throwing it away would leave the ladder with
// nothing to explain.
func Execute(src string, p extract.Program, opts Options) (Result, error) {
	res, err := RunHTML(src, p, opts)
	if err != nil {
		return res, err
	}
	return res, Verify(p, res)
}

// Failures extracts re-synthesis input from an error produced by this package.
// It returns nil for anything else, since an error the runtime does not
// understand is not something to hand a model as a fix instruction.
func Failures(err error) []string {
	var ve *VerifyError
	if errors.As(err, &ve) {
		return ve.Failures()
	}
	var pe *ProgramError
	if errors.As(err, &pe) {
		return pe.Failures()
	}
	return nil
}
