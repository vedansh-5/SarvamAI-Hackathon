package server

import (
	"errors"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/vedansh/vans/internal/synth"
)

const spareKeys = 2

type wordingForm struct {
	Title     string
	Lede      string
	Keys      []synth.Key
	Abilities []abilityForm
}

type abilityForm struct {
	ID       string
	Name     string
	Title    string
	Says     string
	Examples string
	Order    int
	Hidden   bool
}

func wordingFrom(saved synth.Content) wordingForm {
	w := saved.WelcomeOver(synth.BuiltInWelcome())
	f := wordingForm{Title: w.Title, Lede: w.Lede, Keys: append(slices.Clone(w.Keys), make([]synth.Key, spareKeys)...)}
	base := synth.BuiltInAbilities()
	byID := map[string]synth.AbilityText{}
	for _, a := range base {
		byID[a.ID] = a
	}
	over := map[string]synth.AbilityText{}
	var order []string
	for _, o := range saved.Abilities {
		if _, ok := byID[o.ID]; ok {
			if _, dup := over[o.ID]; !dup {
				order = append(order, o.ID)
			}
			over[o.ID] = o
		}
	}
	for _, a := range base {
		if _, ok := over[a.ID]; !ok {
			order = append(order, a.ID)
		}
	}
	for i, id := range order {
		a, o := byID[id], over[id]
		row := abilityForm{ID: id, Name: a.Title, Title: a.Title, Says: a.Says, Examples: strings.Join(a.Examples, "\n"), Order: i + 1, Hidden: o.Hidden}
		if o.Title != "" {
			row.Title = o.Title
		}
		if o.Says != "" {
			row.Says = o.Says
		}
		if len(o.Examples) > 0 {
			row.Examples = strings.Join(o.Examples, "\n")
		}
		f.Abilities = append(f.Abilities, row)
	}
	return f
}

func formWording(form url.Values) wordingForm {
	f := wordingForm{Title: strings.TrimSpace(form.Get("title")), Lede: strings.TrimSpace(form.Get("lede"))}
	before, keys, after := form["key.before"], form["key.key"], form["key.after"]
	for i := range max(len(before), len(keys), len(after)) {
		f.Keys = append(f.Keys, synth.Key{Before: field(before, i), Key: field(keys, i), After: field(after, i)})
	}
	for i, b := range synth.BuiltInAbilities() {
		p := "a." + b.ID + "."
		order, err := strconv.Atoi(strings.TrimSpace(form.Get(p + "order")))
		if err != nil {
			order = i + 1
		}
		f.Abilities = append(f.Abilities, abilityForm{
			ID: b.ID, Name: b.Title, Order: order, Hidden: form.Get(p+"hidden") == "yes",
			Title: strings.TrimSpace(form.Get(p + "title")), Says: strings.TrimSpace(form.Get(p + "says")),
			Examples: strings.TrimSpace(form.Get(p + "examples")),
		})
	}
	return f
}

func field(values []string, i int) string {
	if i < len(values) {
		return strings.TrimSpace(values[i])
	}
	return ""
}

func (f wordingForm) content() (synth.Content, error) {
	var c synth.Content
	base := synth.BuiltInWelcome()
	var w synth.Welcome
	if f.Title != base.Title {
		w.Title = f.Title
	}
	if f.Lede != base.Lede {
		w.Lede = f.Lede
	}
	var keys []synth.Key
	for _, k := range f.Keys {
		if k == (synth.Key{}) {
			continue
		}
		if k.Key == "" {
			return c, errors.New("every key line needs the key itself, such as Esc")
		}
		keys = append(keys, k)
	}
	if !slices.Equal(keys, base.Keys) {
		w.Keys = keys
	}
	if w.Title != "" || w.Lede != "" || len(w.Keys) > 0 {
		c.Welcome = &w
	}

	builtIn := synth.BuiltInAbilities()
	byID := map[string]synth.AbilityText{}
	for _, b := range builtIn {
		byID[b.ID] = b
	}
	rows := slices.Clone(f.Abilities)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Order < rows[j].Order })
	changed := false
	var out []synth.AbilityText
	for i, r := range rows {
		b := byID[r.ID]
		t := synth.AbilityText{ID: r.ID, Hidden: r.Hidden}
		if r.Title != b.Title {
			t.Title = r.Title
		}
		if r.Says != b.Says {
			t.Says = r.Says
		}
		if ex := examplesIn(r.Examples); !slices.Equal(ex, b.Examples) {
			t.Examples = ex
		}
		if i >= len(builtIn) || builtIn[i].ID != r.ID || t.Title != "" || t.Says != "" || len(t.Examples) > 0 || t.Hidden {
			changed = true
		}
		out = append(out, t)
	}
	if changed {
		c.Abilities = out
	}
	return c, nil
}

func examplesIn(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func wordingToKeep(form url.Values) (wordingForm, string, error) {
	f := formWording(form)
	got, err := f.content()
	if err != nil {
		return f, "", err
	}
	raw := ""
	if got.Welcome != nil || got.Abilities != nil {
		raw = got.Spelled()
	}
	if _, err := synth.ParseContent(raw); err != nil {
		return f, "", err
	}
	return f, raw, nil
}
