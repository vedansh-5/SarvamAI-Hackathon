package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/pagemark"
	"github.com/vedansh/vans/internal/synth"
)

func titles(as []Ability) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.Title)
	}
	return out
}

func TestOnlyWhatThisBuildCanDoIsListed(t *testing.T) {
	got := titles((&Engine{}).Abilities(context.Background()))
	for _, missing := range []string{"Show you how", "Do it for you", "Remember things", "Mark things on a page"} {
		for _, title := range got {
			if title == missing {
				t.Errorf("a bare build lists %q, which it cannot do", missing)
			}
		}
	}
}

func TestTeachingComesFirst(t *testing.T) {
	e := &Engine{UIScreen: &fakeScreen{tree: editorTree()}, Steps: &alwaysStep{step: typeIntoMessage()}}
	got := e.Abilities(context.Background())
	if len(got) == 0 || got[0].Title != "Show you how" {
		t.Fatalf("first ability = %v, want Show you how", titles(got))
	}
}

func TestEveryMarkingExampleIsOneVANSUnderstandsWithoutAModel(t *testing.T) {
	e := &Engine{PageScreen: &browserPage{front: "Google Chrome"}}
	for _, a := range e.Abilities(context.Background()) {
		if a.Title != "Mark things on a page" {
			continue
		}
		for _, example := range a.Examples {
			if _, ok := pagemark.Understand(example); !ok {
				t.Errorf("%q is offered as an example but is not understood", example)
			}
		}
		return
	}
	t.Fatal("marking was not listed for a build that can mark pages")
}

func TestEveryAbilitySaysSomethingPlain(t *testing.T) {
	for _, a := range (&Engine{}).Abilities(context.Background()) {
		if a.Says == "" || strings.Contains(a.Says, "AX") {
			t.Errorf("%q says %q", a.Title, a.Says)
		}
	}
}

type wordingFrom struct {
	c     synth.Content
	asked int
}

func (w *wordingFrom) Content(context.Context) (synth.Content, error) {
	w.asked++
	return w.c, nil
}

func TestTheServerCanRewordButNotAddAbilities(t *testing.T) {
	source := &wordingFrom{c: synth.Content{Abilities: []synth.AbilityText{
		{ID: synth.AbilityMemory, Title: "Keep facts"},
		{ID: synth.AbilityGuide, Says: "Points at each step."},
	}}}
	e := &Engine{UIScreen: &fakeScreen{tree: editorTree()}, Steps: &alwaysStep{step: typeIntoMessage()}, Content: Cached(source)}
	got := e.Abilities(context.Background())
	if len(got) == 0 || got[0].ID != synth.AbilityGuide || got[0].Says != "Points at each step." || got[0].Title != "Show you how" {
		t.Fatalf("got %+v", got)
	}
	for _, a := range got {
		if a.ID == synth.AbilityMemory {
			t.Errorf("listed %q, which this build has no memory for", a.Title)
		}
	}
	e.Abilities(context.Background())
	e.Welcome(context.Background())
	if source.asked != 1 {
		t.Errorf("asked the server %d times; the wording is kept for a while", source.asked)
	}
}

func TestMarkingExamplesFromTheServerMustBeUnderstood(t *testing.T) {
	source := &wordingFrom{c: synth.Content{Abilities: []synth.AbilityText{
		{ID: synth.AbilityMarks, Examples: []string{"make the important bits stand out"}},
	}}}
	e := &Engine{PageScreen: &browserPage{front: "Google Chrome"}, Content: Cached(source)}
	for _, a := range e.Abilities(context.Background()) {
		if a.ID != synth.AbilityMarks {
			continue
		}
		for _, example := range a.Examples {
			if _, ok := pagemark.Understand(example); !ok {
				t.Errorf("%q is offered but not understood", example)
			}
		}
		return
	}
	t.Fatal("marking was not listed")
}

func TestTheWelcomeComesFromTheServerWhenItHasOne(t *testing.T) {
	e := &Engine{Content: Cached(&wordingFrom{c: synth.Content{Welcome: &synth.Welcome{Title: "Hi"}}})}
	if w := e.Welcome(context.Background()); w.Title != "Hi" || w.Lede != synth.BuiltInWelcome().Lede {
		t.Errorf("got %+v", w)
	}
	if w := (&Engine{}).Welcome(context.Background()); w.Title != synth.BuiltInWelcome().Title {
		t.Errorf("without a server got %+v", w)
	}
}
