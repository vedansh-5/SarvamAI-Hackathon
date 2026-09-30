package uiact

import (
	"strings"
	"testing"
)

func scrolledList(at int) Tree {
	return Tree{
		App: "Settings", Window: "Settings",
		Elements: []Element{
			{Role: "AXScrollArea", Enabled: true, Position: []int{0, 0}, Size: []int{400, 300}, Path: []int{0}},
			{Role: "AXButton", Title: "Sonoma", Enabled: true, Position: []int{20, 40}, Size: []int{100, 20}, Path: []int{0, 0}},
			{Role: "AXButton", Title: "Big Sur", Enabled: true, Position: []int{20, at}, Size: []int{100, 20}, Path: []int{0, 1}},
		},
	}
}

func TestAControlScrolledAwayIsMarkedForTheModel(t *testing.T) {
	text := scrolledList(600).Text()
	if !strings.Contains(text, `AXButton "Big Sur" [out of view]`) {
		t.Errorf("Big Sur not marked out of view:\n%s", text)
	}
	if strings.Contains(text, `"Sonoma" [out of view]`) {
		t.Errorf("Sonoma marked out of view though it is on screen:\n%s", text)
	}
}

func TestTheScrollHintSaysWhichWayAndWhere(t *testing.T) {
	big := Selector{Role: "AXButton", Title: "Big Sur"}
	for _, c := range []struct {
		at  int
		way string
	}{{600, "down"}, {-200, "up"}} {
		x, y, w, h, way, ok := scrolledList(c.at).ScrollHint(big)
		if !ok || way != c.way {
			t.Errorf("at y=%d: way=%q ok=%v, want %q", c.at, way, ok, c.way)
		}
		if x != 0 || y != 0 || w != 400 || h != 300 {
			t.Errorf("at y=%d: area (%d,%d %dx%d), want the scroll area", c.at, x, y, w, h)
		}
	}
	if _, _, _, _, _, ok := scrolledList(150).ScrollHint(big); ok {
		t.Error("gave a scroll hint for a control already in view")
	}
}
