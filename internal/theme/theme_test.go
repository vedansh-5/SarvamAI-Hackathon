package theme

import (
	"math"
	"testing"
)

func luminance(c string) float64 {
	r, g, b := RGB(c)
	f := func(v int) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(r) + 0.7152*f(g) + 0.0722*f(b)
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func TestEveryThemeHighlightsLightAndReadable(t *testing.T) {
	if len(All()) < 2 {
		t.Fatalf("only %d themes", len(All()))
	}
	for _, th := range All() {
		if l := luminance(th.Soft); l < 0.7 {
			t.Errorf("%s: the highlight %s is not light (luminance %.2f)", th.ID, th.Soft, l)
		}
		if c := contrast(th.Soft, th.Ink); c < 7 {
			t.Errorf("%s: text on the highlight has contrast %.1f, want 7 or more", th.ID, c)
		}
		if c := contrast(th.Accent, th.OnAccent); c < 4.5 {
			t.Errorf("%s: text on the accent has contrast %.1f, want 4.5 or more", th.ID, c)
		}
		if c := contrast(th.Soft, "#ffffff"); c < 1.1 {
			t.Errorf("%s: the highlight barely shows on white (%.2f)", th.ID, c)
		}
	}
}

func TestAnUnknownThemeFallsBackToTheDefault(t *testing.T) {
	if Named("nonsense").ID != Default().ID || Named(" Blue ").ID != "blue" {
		t.Errorf("Named does not fall back or ignore case")
	}
}
