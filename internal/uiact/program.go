package uiact

import "strings"

type Program struct {
	App   string `json:"app"`
	Steps []Step `json:"steps"`
}

type Step struct {
	Do       string   `json:"do"`
	Target   Selector `json:"target"`
	Text     string   `json:"text,omitempty"`
	Key      string   `json:"key,omitempty"`
	Describe string   `json:"describe,omitempty"`
}

type Selector struct {
	Role  string `json:"role"`
	Title string `json:"title,omitempty"`
	Index int    `json:"index,omitempty"`
}

var Verbs = map[string]bool{
	"click": true,
	"type":  true,
	"press": true,
}

var Keys = map[string]int{
	"Enter":  36,
	"Escape": 53,
	"Tab":    48,
}

const MaxSteps = 8

func (p Program) Writes() bool { return len(p.Steps) > 0 }

func (p Program) Values() []string {
	var out []string
	for _, s := range p.Steps {
		if s.Text != "" {
			out = append(out, s.Text)
		}
		if s.Target.Title != "" {
			out = append(out, s.Target.Title)
		}
	}
	return out
}

func (p Program) Fill(replace func(string) string) Program {
	out := Program{App: p.App, Steps: make([]Step, len(p.Steps))}
	for i, s := range p.Steps {
		s.Text = replace(s.Text)
		s.Describe = replace(s.Describe)
		s.Target.Title = replace(s.Target.Title)
		out.Steps[i] = s
	}
	return out
}

func verb(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
