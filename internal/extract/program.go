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
	// Root selects every record in the region, relative to the document.
	Root string `json:"root"`

	// Fields are extracted from each record, with selectors relative to Root.
	Fields []Field `json:"fields"`

	// Expect is the model-authored success condition. The runtime enforces it;
	// it does not author it.
	Expect Expectation `json:"expect"`
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
		"required":             []string{"root", "fields", "expect"},
		"properties": map[string]any{
			"root": map[string]any{
				"type":        "string",
				"description": "CSS selector matching every record in the region, relative to the document.",
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
