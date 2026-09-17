package webstyle

import (
	"strings"
	"testing"
)

func TestStructureFindsTheRepeatingParts(t *testing.T) {
	parts := Structure(page(t, shopHTML))

	var found bool
	for _, p := range parts {
		if p.Selector == "article.card" && p.Count == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("did not find the two cards: %+v", parts)
	}
}

// The page in front of someone is as likely to be their mail as a shop, and a
// restyle decides which elements change rather than what they say.
func TestStructureCarriesNoPageText(t *testing.T) {
	got := StructureText(Structure(page(t, shopHTML)), MaxStructureBytes)
	for _, secret := range []string{"$250", "$80", "Widget", "Gadget"} {
		if strings.Contains(got, secret) {
			t.Errorf("page text reached the summary: %q is in\n%s", secret, got)
		}
	}
	if !strings.Contains(got, "span.price") {
		t.Errorf("the structure itself did not travel:\n%s", got)
	}
}

func TestStructureSkipsGeneratedClasses(t *testing.T) {
	parts := Structure(page(t, `<div class="css-1a2b3c4d">x</div>`))
	for _, p := range parts {
		if strings.Contains(p.Selector, "css-1a2b3c4d") {
			t.Errorf("a hashed class was offered: %+v", parts)
		}
	}
}
