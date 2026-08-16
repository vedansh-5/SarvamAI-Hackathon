package extract

import (
	"fmt"
	"strings"
)

// Program is an extraction program: a selector-based recipe for pulling
// structured rows out of a page.
//
// This is the artifact the whole caching thesis rests on. The model writes one
// per (intent, page shape) and the runtime replays it forever after — so a
// Program has to be executable without any model involvement, and it has to
// carry its own success condition.
type Program struct {
	// Root selects the first element of every record, relative to the document.
	Root string `json:"root"`

	// RootSpan is how many consecutive sibling elements make up one record,
	// counting the one Root matched. Zero and one both mean a single element.
	//
	// This exists because a data record is not always one subtree. MDR (which
	// the candidate finder follows) defines a record as a *generalized node* —
	// a set of consecutive sibling subtrees — and table-based layouts use that
	// shape constantly. Hacker News puts a story's title in one <tr> and its
	// score in the next; with a single-element record the score is not merely
	// hard to select, it is unreachable, because every field selector is
	// evaluated relative to the record.
	RootSpan int `json:"root_span"`

	// Fields are extracted from each record, with selectors relative to it.
	Fields []Field `json:"fields"`

	// Expect is the model-authored success condition. The runtime enforces it;
	// it does not author it.
	Expect Expectation `json:"expect"`
}

// MaxRootSpan bounds how many siblings one record may cover.
//
// A record made of many consecutive siblings is more likely a mis-detected
// region than a real one, and an unbounded span lets a single root swallow the
// whole page.
const MaxRootSpan = 6

// Span is RootSpan normalized: always at least 1.
//
// Programs cached before spans existed decode with RootSpan zero, and they
// must keep working — every one of them describes a single-element record,
// which is exactly what 1 means.
func (p Program) Span() int {
	if p.RootSpan < 1 {
		return 1
	}
	return p.RootSpan
}

// Field is one column of the extracted data.
type Field struct {
	Name string `json:"name"`

	// Selector is relative to the record root. Empty means the record itself.
	Selector string `json:"selector"`

	// Attr is what to read: "text" or an attribute name.
	Attr string `json:"attr"`
}

// Expectation is the contract a successful run must satisfy.
type Expectation struct {
	MinRows int `json:"min_rows"`

	// Required names fields that must be non-empty in every row.
	Required []string `json:"required"`
}

// AllowedAttrs are the values Attr may take. Restricting the set keeps the
// runner deterministic and keeps the model from inventing extraction verbs the
// runtime does not implement.
var AllowedAttrs = []string{"text", "href", "src", "title", "value", "alt", "datetime"}

func attrAllowed(a string) bool {
	for _, v := range AllowedAttrs {
		if v == a {
			return true
		}
	}
	return false
}

// Validate checks a program is executable before it is ever run or cached.
//
// This runs on synthesized programs and on programs loaded from cache — a
// stored program can be invalid if the schema changed since it was written.
func (p Program) Validate() error {
	if strings.TrimSpace(p.Root) == "" {
		return fmt.Errorf("program: root selector is empty")
	}
	if len(p.Fields) == 0 {
		return fmt.Errorf("program: no fields")
	}
	if p.RootSpan < 0 || p.RootSpan > MaxRootSpan {
		return fmt.Errorf("program: root_span is %d, want 0 to %d", p.RootSpan, MaxRootSpan)
	}

	seen := make(map[string]bool, len(p.Fields))
	for i, f := range p.Fields {
		if strings.TrimSpace(f.Name) == "" {
			return fmt.Errorf("program: field %d has no name", i)
		}
		if seen[f.Name] {
			return fmt.Errorf("program: duplicate field %q", f.Name)
		}
		seen[f.Name] = true

		if !attrAllowed(f.Attr) {
			return fmt.Errorf("program: field %q has attr %q, want one of %v", f.Name, f.Attr, AllowedAttrs)
		}
	}

	if p.Expect.MinRows < 1 {
		return fmt.Errorf("program: expect.min_rows is %d, want at least 1", p.Expect.MinRows)
	}
	for _, name := range p.Expect.Required {
		if !seen[name] {
			return fmt.Errorf("program: expect.required names %q, which is not a field", name)
		}
	}
	return nil
}

// FieldNames returns the field names in order.
func (p Program) FieldNames() []string {
	out := make([]string, len(p.Fields))
	for i, f := range p.Fields {
		out[i] = f.Name
	}
	return out
}

// ProgramSchema is the JSON Schema handed to the model for structured output.
//
// Every object sets additionalProperties:false and lists required — the API's
// structured-output support requires both. Numeric and string constraints are
// deliberately absent; they are unsupported there and are enforced by Validate
// instead.
func ProgramSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"root", "root_span", "fields", "expect"},
		"properties": map[string]any{
			"root": map[string]any{
				"type":        "string",
				"description": "CSS selector matching the first element of every record, relative to the document.",
			},
			"root_span": map[string]any{
				"type": "integer",
				"description": "How many consecutive sibling elements make up one record, counting the one root matched. " +
					"Use 1 when each record is a single element. Use the span shown for the candidate when a record " +
					"is split across siblings, as in a table that puts a row's details in the following row.",
			},
			"fields": map[string]any{
				"type":        "array",
				"description": "One entry per column to extract from each record.",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"name", "selector", "attr"},
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Short snake_case column name.",
						},
						"selector": map[string]any{
							"type":        "string",
							"description": "CSS selector relative to the record root. Empty string means the record element itself.",
						},
						"attr": map[string]any{
							"type":        "string",
							"enum":        AllowedAttrs,
							"description": "What to read from the matched element: \"text\" for its visible text, otherwise an attribute name.",
						},
					},
				},
			},
			"expect": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"min_rows", "required"},
				"description":          "The success condition the runtime will enforce on every future run of this program.",
				"properties": map[string]any{
					"min_rows": map[string]any{
						"type":        "integer",
						"description": "Fewest rows a correct run may produce. At least 1.",
					},
					"required": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Field names that must be non-empty in every row.",
					},
				},
			},
		},
	}
}
