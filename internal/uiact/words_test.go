package uiact

import "testing"

func TestWordsFarApartAreSeparateLabelsAndCloseOnesOne(t *testing.T) {
	page := seenPage{Width: 1000, Height: 500, Lines: []seenLine{
		{Text: "Cut   Copy", Confidence: 1, Words: []seenWord{
			{At: 0, Len: 3, X: 0.10, Y: 0.1, W: 0.03, H: 0.03},
			{At: 6, Len: 4, X: 0.20, Y: 0.1, W: 0.04, H: 0.03},
		}},
		{Text: "Render Queue", Confidence: 1, Words: []seenWord{
			{At: 0, Len: 6, X: 0.50, Y: 0.2, W: 0.06, H: 0.03},
			{At: 7, Len: 5, X: 0.565, Y: 0.2, W: 0.05, H: 0.03},
		}},
		{Text: "Export…", Confidence: 1, Words: []seenWord{{At: 0, Len: 6, X: 0.1, Y: 0.3, W: 0.06, H: 0.03}}},
		{Text: "smudge", Confidence: 0.1, Words: []seenWord{{At: 0, Len: 6, X: 0.1, Y: 0.4, W: 0.06, H: 0.03}}},
	}}
	got := pieces(page)
	var texts []string
	for _, w := range got {
		texts = append(texts, w.Text)
	}
	want := []string{"Cut", "Copy", "Render Queue", "Export…"}
	if len(texts) != len(want) {
		t.Fatalf("pieces = %q, want %q", texts, want)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Errorf("piece %d = %q, want %q", i, texts[i], want[i])
		}
	}
	if rq := got[2]; rq.X != 0.50 || rq.W < 0.11 {
		t.Errorf("Render Queue box = %+v, want it to span both words", rq)
	}
}

func TestLookalikeLettersAreReadAsLatin(t *testing.T) {
	if got := latin("Сopy"); got != "Copy" {
		t.Errorf("latin = %q", got)
	}
	if got := latin("Привет"); got != "Привет" {
		t.Errorf("a Russian word was changed: %q", got)
	}
	if !SameLabel("Export...", "Export…") || !SameLabel(" Save  As ", "save as") || SameLabel("Save", "Save As") || !SameLabel("Down load", "Download") {
		t.Error("SameLabel compares the wrong way")
	}
}
