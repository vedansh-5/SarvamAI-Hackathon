package names

import "testing"

func TestHiddenCharactersDoNotChangeAName(t *testing.T) {
	mark := "\u200e"
	if got := Clean(mark + "WhatsApp"); got != "WhatsApp" {
		t.Errorf("Clean = %q, want the name without the mark in front of it", got)
	}
	if !Same("whatsapp", mark+"WhatsApp") {
		t.Error("whatsapp and the name WhatsApp ships under were treated as different applications")
	}
	if !Same("  Google   Chrome ", "google chrome") {
		t.Error("spacing and case changed which application a name means")
	}
	if Same("Notes", "Notion") {
		t.Error("two different applications were treated as the same")
	}
	if got := Clean("Caf\u00e9\u200b Notes\ufeff"); got != "Caf\u00e9 Notes" {
		t.Errorf("Clean = %q, want accents kept and only the hidden characters gone", got)
	}
}
