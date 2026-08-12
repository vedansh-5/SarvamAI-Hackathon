package extract

import (
	"encoding/json"
	"strings"
	"testing"
)

func validProgram() Program {
	return Program{
		Root: "div.results > article.card",
		Fields: []Field{
			{Name: "title", Selector: "h3 a", Attr: "text"},
			{Name: "url", Selector: "h3 a", Attr: "href"},
			{Name: "price", Selector: ".price", Attr: "text"},
		},
		Expect: Expectation{MinRows: 1, Required: []string{"title", "url"}},
	}
}

func TestValidProgramPasses(t *testing.T) {
	if err := validProgram().Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// Validate is the gate between the model and the cache: an invalid program
// must never be stored, or every later run replays it.
func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Program){
		"empty root":         func(p *Program) { p.Root = "  " },
		"no fields":          func(p *Program) { p.Fields = nil },
		"unnamed field":      func(p *Program) { p.Fields[0].Name = "" },
		"duplicate field":    func(p *Program) { p.Fields[1].Name = p.Fields[0].Name },
		"unknown attr":       func(p *Program) { p.Fields[0].Attr = "innerHTML" },
		"zero min_rows":      func(p *Program) { p.Expect.MinRows = 0 },
		"required not field": func(p *Program) { p.Expect.Required = []string{"nope"} },
	}

	for name, mutate := range cases {
		p := validProgram()
		mutate(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: Validate accepted an invalid program", name)
		}
	}
}

// An empty selector is meaningful — it reads the record element itself.
func TestEmptySelectorIsValid(t *testing.T) {
	p := validProgram()
	p.Fields[0].Selector = ""
	if err := p.Validate(); err != nil {
		t.Errorf("Validate rejected an empty selector: %v", err)
	}
}

func TestProgramRoundTripsJSON(t *testing.T) {
	want := validProgram()

	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Program
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Root != want.Root || len(got.Fields) != len(want.Fields) {
		t.Errorf("round trip changed the program: %+v", got)
	}
	if !strings.Contains(string(b), `"min_rows"`) {
		t.Errorf("expect block did not use snake_case JSON names: %s", b)
	}
}

// The structured-outputs API rejects any object that omits
// additionalProperties:false or required, and it does so at request time — so
// a malformed schema fails every synthesis, not just an edge case.
func TestSchemaMeetsStructuredOutputRules(t *testing.T) {
	var walk func(path string, node map[string]any)
	walk = func(path string, node map[string]any) {
		if node["type"] == "object" {
			if node["additionalProperties"] != false {
				t.Errorf("%s: object without additionalProperties:false", path)
			}
			if _, ok := node["required"]; !ok {
				t.Errorf("%s: object without required", path)
			}
			props, _ := node["properties"].(map[string]any)
			for name, child := range props {
				if c, ok := child.(map[string]any); ok {
					walk(path+"."+name, c)
				}
			}
		}
		if node["type"] == "array" {
			if items, ok := node["items"].(map[string]any); ok {
				walk(path+"[]", items)
			}
		}
	}
	walk("root", ProgramSchema())
}

func TestSchemaIsJSONSerializable(t *testing.T) {
	if _, err := json.Marshal(ProgramSchema()); err != nil {
		t.Fatalf("schema does not marshal: %v", err)
	}
}

// The runner only implements the attributes in AllowedAttrs, so the schema's
// enum and the validator have to agree — otherwise the model can be handed a
// value that Validate then rejects on arrival.
func TestSchemaEnumMatchesValidator(t *testing.T) {
	fields := ProgramSchema()["properties"].(map[string]any)["fields"].(map[string]any)
	item := fields["items"].(map[string]any)
	attr := item["properties"].(map[string]any)["attr"].(map[string]any)

	enum, ok := attr["enum"].([]string)
	if !ok {
		t.Fatalf("attr enum is %T, want []string", attr["enum"])
	}
	if len(enum) != len(AllowedAttrs) {
		t.Fatalf("enum has %d values, AllowedAttrs has %d", len(enum), len(AllowedAttrs))
	}
	for _, v := range enum {
		if !attrAllowed(v) {
			t.Errorf("schema offers attr %q that Validate rejects", v)
		}
	}
}

func TestFieldNames(t *testing.T) {
	got := validProgram().FieldNames()
	want := []string{"title", "url", "price"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}
