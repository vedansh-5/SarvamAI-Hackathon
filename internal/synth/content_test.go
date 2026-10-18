package synth

import (
	"strings"
	"testing"
)

func ids(as []AbilityText) string {
	var out []string
	for _, a := range as {
		out = append(out, a.ID)
	}
	return strings.Join(out, ",")
}

func TestTheBuiltInWordingReadsBackAsItself(t *testing.T) {
	c, err := ParseContent(BuiltInContent().Spelled())
	if err != nil {
		t.Fatal(err)
	}
	if ids(c.Abilities) != ids(BuiltInAbilities()) || c.Welcome.Title != BuiltInWelcome().Title {
		t.Errorf("read back %+v", c)
	}
}

func TestWordingThatCannotBeUsedIsTurnedDown(t *testing.T) {
	for _, raw := range []string{
		`{"welcome":`,
		`{"welcome":{"tite":"Hi"}}`,
		`{"abilities":[{"id":"teleport"}]}`,
		`{"abilities":[{"id":"files"},{"id":"files"}]}`,
		`{"welcome":{"keys":[{"after":"opens VANS"}]}}`,
		`{} {}`,
	} {
		if _, err := ParseContent(raw); err == nil {
			t.Errorf("%s was accepted", raw)
		}
	}
	if c, err := ParseContent("  "); err != nil || c.Welcome != nil {
		t.Errorf("empty text gave %+v, %v", c, err)
	}
}

func TestTheServerChangesWordingAndOrderButCannotAddAnAbility(t *testing.T) {
	base := []AbilityText{
		{ID: AbilityGuide, Title: "Show you how", Says: "a", Examples: []string{"x"}},
		{ID: AbilityFiles, Title: "Tidy files", Says: "b"},
		{ID: AbilityOnText, Title: "Work on text", Says: "c"},
	}
	c := Content{Abilities: []AbilityText{
		{ID: AbilityOnText, Says: "changed"},
		{ID: AbilityFiles, Hidden: true},
		{ID: AbilityMemory, Title: "Remember things"},
	}}
	got := c.AbilitiesOver(base)
	if ids(got) != "text,guide" {
		t.Fatalf("got %s, want text first, files hidden, memory not added", ids(got))
	}
	if got[0].Says != "changed" || got[0].Title != "Work on text" || got[1].Examples[0] != "x" {
		t.Errorf("got %+v; a field left out keeps the built-in one", got)
	}
}

func TestTheWelcomeKeepsWhatIsNotChanged(t *testing.T) {
	w := Content{Welcome: &Welcome{Lede: "new"}}.WelcomeOver(BuiltInWelcome())
	if w.Lede != "new" || w.Title != BuiltInWelcome().Title || len(w.Keys) != len(BuiltInWelcome().Keys) {
		t.Errorf("got %+v", w)
	}
}

func TestTheWelcomeDoesNotSendPeopleToEscForMarks(t *testing.T) {
	for _, k := range BuiltInWelcome().Keys {
		if strings.Contains(k.After, "mark") {
			t.Errorf("%s: %q; marks come off by asking, Esc needs the page focused", k.Key, k.After)
		}
	}
}

func TestEveryTesterHasAVersionOfEveryTrial(t *testing.T) {
	got := TrialsFor(map[string]string{TrialSetup: "nonsense", "gone": "x"})
	if got[TrialSetup] != SetupOld || len(got) != len(KnownTrials()) {
		t.Errorf("got %v", got)
	}
	if back := TrialsSpelled(got.Spelled()); back[TrialSetup] != SetupOld {
		t.Errorf("read back %v", back)
	}
}
