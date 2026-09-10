// Package appact is a program that acts inside a macOS application.
//
// The program is data rather than code. A model that wrote JXA directly would
// hand the runtime a string it can only check by reading, and reading
// arbitrary JavaScript well enough to say what it will do to an application is
// not something a validator can be trusted to do. So the model chooses from
// the vocabulary the application declares (D-040, D-041), the runtime renders
// the script, and every claim the program makes is checked against the
// dictionary before anything runs.
//
// This is the selector program (D-002) one tier up: the same trade of
// expressiveness for a thing that can be validated, cached, and replayed.
package appact

// Program is one action inside one application.
type Program struct {
	App   string `json:"app"`
	Steps []Step `json:"steps"`
}

// Step is a single command sent to the application.
type Step struct {
	// Command is the verb, and must be one the application declares or one
	// the object model provides.
	Command string `json:"command"`

	// Target is what the command acts on. Empty means the application itself.
	Target *Selector `json:"target,omitempty"`

	// Make names the class to create, for the make command.
	Make string `json:"make,omitempty"`

	// With carries properties: what to create the object with, or what to set
	// on it. Values are literals — the program never computes.
	With map[string]string `json:"with,omitempty"`

	// Bind names this step's result so a later step can address it.
	Bind string `json:"bind,omitempty"`
}

// Selector addresses an object without naming its position in a script.
type Selector struct {
	// Class is the kind of object, singular, as the dictionary spells it.
	Class string `json:"class"`

	// In is the binding of a previous step's result to look inside. Empty
	// means the application.
	In string `json:"in,omitempty"`

	// Name selects by the object's name, which is how a person refers to one.
	Name string `json:"name,omitempty"`

	// Index selects positionally, one-based, when there is nothing to name.
	Index int `json:"index,omitempty"`
}

// Commands lists every verb the program invokes.
func (p Program) Commands() []string {
	out := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		out = append(out, s.Command)
	}
	return out
}

// Classes lists every class the program names, whether as a target or as
// something to create.
func (p Program) Classes() []string {
	var out []string
	for _, s := range p.Steps {
		if s.Make != "" {
			out = append(out, s.Make)
		}
		if s.Target != nil && s.Target.Class != "" {
			out = append(out, s.Target.Class)
		}
	}
	return out
}

// Values lists every literal value the program carries.
func (p Program) Values() []string {
	var out []string
	for _, s := range p.Steps {
		for _, v := range s.With {
			out = append(out, v)
		}
		if s.Target != nil && s.Target.Name != "" {
			out = append(out, s.Target.Name)
		}
	}
	return out
}

// Fill replaces placeholders with this instruction's values.
//
// Only values are filled. Commands, classes and bindings came from the
// dictionary and are not the instruction's to change — a placeholder there
// would be a program rewriting its own structure per request.
func (p Program) Fill(replace func(string) string) Program {
	out := Program{App: p.App, Steps: make([]Step, len(p.Steps))}
	for i, s := range p.Steps {
		step := Step{Command: s.Command, Make: s.Make, Bind: s.Bind}
		if s.Target != nil {
			t := *s.Target
			t.Name = replace(t.Name)
			step.Target = &t
		}
		if s.With != nil {
			step.With = make(map[string]string, len(s.With))
			for k, v := range s.With {
				step.With[k] = replace(v)
			}
		}
		out.Steps[i] = step
	}
	return out
}
