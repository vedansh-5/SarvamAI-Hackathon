package engine

import (
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/pagemark"
)

func titles(as []Ability) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.Title)
	}
	return out
}

func TestOnlyWhatThisBuildCanDoIsListed(t *testing.T) {
	got := titles((&Engine{}).Abilities())
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
	got := e.Abilities()
	if len(got) == 0 || got[0].Title != "Show you how" {
		t.Fatalf("first ability = %v, want Show you how", titles(got))
	}
}

func TestEveryMarkingExampleIsOneVANSUnderstandsWithoutAModel(t *testing.T) {
	e := &Engine{PageScreen: &browserPage{front: "Google Chrome"}}
	for _, a := range e.Abilities() {
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
	for _, a := range (&Engine{}).Abilities() {
		if a.Says == "" || strings.Contains(a.Says, "AX") {
			t.Errorf("%q says %q", a.Title, a.Says)
		}
	}
}
