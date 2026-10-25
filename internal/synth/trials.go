package synth

import (
	"fmt"
	"sort"
	"strings"
)

const KindTrials = "trials"

type Trial struct {
	Name     string
	Says     string
	Variants []string
}

var running []Trial

func KnownTrials() []Trial {
	return running
}

func RunTrials(list []Trial) (restore func()) {
	was := running
	running = list
	return func() { running = was }
}

func TrialNamed(name string) (Trial, bool) {
	for _, t := range KnownTrials() {
		if t.Name == name {
			return t, true
		}
	}
	return Trial{}, false
}

func CheckTrial(name, variant string) error {
	t, ok := TrialNamed(name)
	if !ok {
		return fmt.Errorf("%q is not a trial VANS runs", name)
	}
	for _, v := range t.Variants {
		if v == variant {
			return nil
		}
	}
	return fmt.Errorf("%q is not a version of %s; it has %s", variant, t.Says, strings.Join(t.Variants, " and "))
}

type Trials map[string]string

func TrialsFor(chosen map[string]string) Trials {
	out := Trials{}
	for _, t := range KnownTrials() {
		out[t.Name] = t.Variants[0]
		if v := chosen[t.Name]; CheckTrial(t.Name, v) == nil {
			out[t.Name] = v
		}
	}
	return out
}

func (t Trials) Spelled() string {
	names := make([]string, 0, len(t))
	for name := range t {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+t[name])
	}
	return strings.Join(parts, " ")
}

func TrialsSpelled(spelled string) Trials {
	out := Trials{}
	for _, part := range strings.Fields(spelled) {
		if name, variant, ok := strings.Cut(part, "="); ok {
			out[name] = variant
		}
	}
	return out
}
