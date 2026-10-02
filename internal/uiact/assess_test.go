package uiact

import (
	"fmt"
	"strings"
	"testing"
)

func windowWith(named, unnamed int) Tree {
	t := Tree{App: "Thing"}
	for i := 0; i < named; i++ {
		t.Elements = append(t.Elements, Element{Role: "AXButton", Title: fmt.Sprintf("Button %d", i), Enabled: true})
	}
	for i := 0; i < unnamed; i++ {
		t.Elements = append(t.Elements, Element{Role: "AXButton", Enabled: true})
	}
	t.Elements = append(t.Elements, Element{Role: "AXGroup"}, Element{Role: "AXStaticText", Title: "words"})
	return t
}

func TestAnAppIsJudgedByHowManyOfItsControlsCanBeNamed(t *testing.T) {
	for _, c := range []struct {
		named, unnamed int
		want           string
	}{
		{20, 2, Ready},
		{10, 8, Partly},
		{3, 30, Blind},
		{2, 0, Blind},
		{0, 0, Blind},
	} {
		got := Assess(windowWith(c.named, c.unnamed))
		if got.Verdict != c.want {
			t.Errorf("%d named, %d unnamed: %s (%s), want %s", c.named, c.unnamed, got.Verdict, got.Why, c.want)
		}
		if got.Controls != c.named+c.unnamed {
			t.Errorf("counted %d controls, want %d; groups and text are not controls", got.Controls, c.named+c.unnamed)
		}
	}
}

func TestARowIsNamedAfterTheTextInsideIt(t *testing.T) {
	els := []Element{
		{Role: "AXRow", Path: []int{0}},
		{Role: "AXCell", Path: []int{0, 0}},
		{Role: "AXStaticText", Value: "Shopping list", Path: []int{0, 0, 0}},
		{Role: "AXStaticText", Value: "Yesterday", Path: []int{0, 0, 1}},
		{Role: "AXRow", Path: []int{1}},
		{Role: "AXButton", Title: "Delete", Path: []int{1, 0}},
		{Role: "AXButton", Title: "Kept", Path: []int{2}},
		{Role: "AXStaticText", Value: "Other words", Path: []int{2, 0}},
		{Role: "AXButton", Path: []int{3}},
		{Role: "AXStaticText", Value: "Not inside the button", Path: []int{4}},
	}
	NameByContents(els)

	if els[0].Title != "Shopping list" || els[1].Title != "Shopping list" {
		t.Errorf("row %q, cell %q; want both named after the first text inside them", els[0].Title, els[1].Title)
	}
	if els[4].Title != "" {
		t.Errorf("a row holding only a button was named %q; the button has its own name", els[4].Title)
	}
	if els[6].Title != "Kept" {
		t.Errorf("a control with a name was renamed %q", els[6].Title)
	}
	if els[8].Title != "" {
		t.Errorf("a button was named %q after text that is not inside it", els[8].Title)
	}
}

func TestAMenuBarDoesNotMakeAnUnreadableWindowLookReadable(t *testing.T) {
	tree := Tree{App: "Thing"}
	for i, name := range []string{"Apple", "Thing", "File", "Edit", "View", "Window", "Help"} {
		tree.Elements = append(tree.Elements, Element{Role: "AXMenuBarItem", Title: name, Enabled: true, Path: []int{-1, i}})
	}
	if got := Assess(tree); got.Verdict != Blind || got.Controls != 0 {
		t.Errorf("verdict %s with %d controls; every application has a menu bar, and it says nothing about its window", got.Verdict, got.Controls)
	}
}

func TestAMenuCountsAsOpenOnlyWhenItsRowsAreLaidOut(t *testing.T) {
	stacked := []Element{{Position: []int{10, 30}}, {Position: []int{10, 52}}, {Position: []int{10, 74}}}
	if !menuIsOpen(stacked) {
		t.Error("a menu with rows one under another was not treated as open")
	}
	piled := []Element{{Position: []int{0, 0}}, {Position: []int{0, 0}}, {Position: []int{0, 0}}}
	if menuIsOpen(piled) {
		t.Error("a closed menu, whose rows all report the same place, was treated as open")
	}
	if menuIsOpen(nil) {
		t.Error("no rows was treated as an open menu")
	}
}

func TestAVeryLargeWindowKeepsItsControlsAndDropsItsFiller(t *testing.T) {
	var els []Element
	for i := 0; i < 600; i++ {
		els = append(els, Element{Role: "AXGroup", Path: []int{i}, Size: []int{100, 20}})
		els = append(els, Element{Role: "AXStaticText", Value: "some words", Path: []int{i, 0}, Size: []int{100, 20}})
	}
	els = append(els,
		Element{Role: "AXRadioButton", Title: "Explorer", Enabled: true, Path: []int{700}, Position: []int{0, 73}, Size: []int{42, 42}},
		Element{Role: "AXButton", Title: "Collapsed away", Enabled: true, Path: []int{701}, Position: []int{0, 504}, Size: []int{1, 29}},
		Element{Role: "AXGroup", Title: "Sidebar", Path: []int{702}, Size: []int{200, 600}},
		Element{Role: "AXMenuBarItem", Title: "File", Enabled: true, Path: []int{-1, 2}, Size: []int{40, 22}},
	)
	kept := Slim(els, 700)

	names := map[string]bool{}
	for _, el := range kept {
		names[el.Label()] = true
	}
	if len(kept) != 3 || !names["Explorer"] || !names["Sidebar"] || !names["File"] {
		t.Errorf("kept %d: %v; want the control, the named container and the menu, and none of the filler", len(kept), names)
	}
	if names["Collapsed away"] {
		t.Error("kept a control one point wide, which nobody can see or click")
	}

	small := []Element{{Role: "AXGroup"}, {Role: "AXStaticText", Value: "words"}}
	if got := Slim(small, 700); len(got) != 2 {
		t.Errorf("a small window was thinned to %d element(s); only a very large one needs it", len(got))
	}
}

func TestWhatIsOnScreenIsKeptAheadOfWhatHasScrolledAway(t *testing.T) {
	was := displays
	displays = func() (box, bool) { return box{0, 0, 1700, 1100}, true }
	t.Cleanup(func() { displays = was })

	var els []Element
	for i := 0; i < 400; i++ {
		els = append(els, Element{Role: "AXCell", Title: "old row", Enabled: true, Path: []int{i}, Position: []int{100, -40000 + i}, Size: []int{500, 25}})
	}
	els = append(els, Element{Role: "AXButton", Title: "Settings", Enabled: true, Path: []int{900}, Position: []int{1545, 1070}, Size: []int{130, 22}})

	kept := Slim(els, 50)
	if len(kept) != 50 || kept[0].Title != "Settings" {
		t.Fatalf("kept %d, first is %q; the one control on screen must come before four hundred that are not", len(kept), kept[0].Title)
	}
}

func TestAnOpenMenuIsReadBeforeAnythingElseSoItIsNeverCutOff(t *testing.T) {
	menus := []Element{
		{Role: "AXMenuBarItem", Title: "File", Enabled: true, Path: []int{-1, 2}, Size: []int{40, 22}},
		{Role: "AXMenuItem", Title: "Settings\u2026", Enabled: true, Path: []int{-1, 1, 0, 3}, Position: []int{38, 103}, Size: []int{242, 24}},
	}
	var window []Element
	for i := 0; i < 900; i++ {
		window = append(window, Element{Role: "AXButton", Title: "Row", Enabled: true, Path: []int{i}, Position: []int{10, 10}, Size: []int{50, 20}})
	}

	all := MenusFirst(menus, window)
	if all[0].Role != "AXMenuItem" || all[1].Role != "AXMenuBarItem" {
		t.Fatalf("first two are %s and %s; an open menu's rows come first, then the menu bar", all[0].Role, all[1].Role)
	}

	kept := Slim(all, 700)
	if kept[0].Title != "Settings\u2026" {
		t.Errorf("after thinning the first is %q", kept[0].Title)
	}
	text := Tree{App: "Thing", Elements: kept}.TextWithin(2000)
	if !strings.Contains(text, "Settings") {
		t.Errorf("the open menu did not survive a tight budget:\n%s", text[:200])
	}
}

func openAppMenu() Tree {
	return Tree{App: "Thing", Elements: []Element{
		{Role: "AXMenuItem", Title: "About Thing", Enabled: true, Path: []int{-1, 1, 0, 0}, Position: []int{38, 44}, Size: []int{242, 24}},
		{Role: "AXMenuItem", Title: "Settings\u2026", Enabled: true, Path: []int{-1, 1, 0, 3}, Position: []int{38, 103}, Size: []int{242, 24}},
		{Role: "AXMenuItem", Title: "Quit Thing", Enabled: true, Path: []int{-1, 1, 0, 11}, Position: []int{38, 256}, Size: []int{242, 24}},
		{Role: "AXMenuBarItem", Title: "Thing", Enabled: true, Path: []int{-1, 1}, Position: []int{43, 0}, Size: []int{118, 24}},
	}}
}

func TestARowOfAnOpenMenuIsInViewThoughItHangsBelowItsMenuBarItem(t *testing.T) {
	was := displays
	displays = func() (box, bool) { return box{0, 0, 1700, 1100}, true }
	t.Cleanup(func() { displays = was })

	tree := openAppMenu()
	settings := Selector{Role: "AXMenuItem", Title: "Settings\u2026"}
	if tree.OutOfView(settings) {
		t.Fatal("a row of an open menu was called out of view; it hangs below the menu bar item, not inside it")
	}
	x, y, w, h, why := tree.RectFor(settings)
	if why != "" || x != 38 || y != 103 || w != 242 || h != 24 {
		t.Errorf("rect = (%d,%d %dx%d) %q, want the row where it is", x, y, w, h, why)
	}
}

func TestTheOpenMenuIsKnownAsOneBoxToKeepClearOf(t *testing.T) {
	x, y, w, h, ok := openAppMenu().OpenMenuBox()
	if !ok || x != 38 || y != 44 || w != 242 || h != 236 {
		t.Errorf("box = (%d,%d %dx%d) ok=%v, want the whole menu", x, y, w, h, ok)
	}
	if _, _, _, _, ok := (Tree{Elements: []Element{{Role: "AXMenuBarItem", Title: "File", Path: []int{-1, 2}, Position: []int{0, 0}, Size: []int{40, 22}}}}).OpenMenuBox(); ok {
		t.Error("a closed menu bar was reported as an open menu")
	}
}

func TestWithAMenuOpenTheModelIsGivenTheMenuAndNotTheWindowBehindIt(t *testing.T) {
	was := displays
	displays = func() (box, bool) { return box{0, 0, 1700, 1100}, true }
	t.Cleanup(func() { displays = was })

	tree := openAppMenu()
	for i := 0; i < 400; i++ {
		tree.Elements = append(tree.Elements, Element{Role: "AXButton", Title: fmt.Sprintf("Window button %d", i), Enabled: true, Path: []int{i}, Position: []int{10, 300}, Size: []int{50, 20}})
	}
	text := tree.Text()

	if !strings.Contains(text, "Settings") || !strings.Contains(text, "A menu is open") {
		t.Fatalf("the open menu is missing from:\n%s", text[:300])
	}
	if strings.Contains(text, "Window button") {
		t.Error("the window behind the menu was listed, though none of it can be used while the menu is open")
	}
	if len(text) > 1000 {
		t.Errorf("%d bytes for a four-row menu; this is the call that was taking five seconds", len(text))
	}
}

func TestALargeWindowListsWhatIsOnScreenAndOnlyCountsTheRest(t *testing.T) {
	was := displays
	displays = func() (box, bool) { return box{0, 0, 1700, 1100}, true }
	t.Cleanup(func() { displays = was })

	tree := Tree{App: "Thing"}
	for i := 0; i < 40; i++ {
		tree.Elements = append(tree.Elements, Element{Role: "AXButton", Title: fmt.Sprintf("Here %d", i), Enabled: true, Path: []int{i}, Position: []int{10, 10 + i*20}, Size: []int{50, 18}})
	}
	for i := 0; i < 400; i++ {
		tree.Elements = append(tree.Elements, Element{Role: "AXCell", Title: fmt.Sprintf("Old row %d", i), Enabled: true, Path: []int{1000 + i}, Position: []int{100, -40000 + i*25}, Size: []int{500, 24}})
	}
	text := tree.Text()

	if strings.Count(text, "Here ") != 40 {
		t.Errorf("listed %d of the 40 controls on screen", strings.Count(text, "Here "))
	}
	if n := strings.Count(text, "Old row"); n != MostOutOfViewListed {
		t.Errorf("listed %d scrolled-away rows, want %d and a count of the rest", n, MostOutOfViewListed)
	}
	if !strings.Contains(text, "375 more that are scrolled out of view") {
		t.Errorf("the rest were not counted:\n%s", text[len(text)-200:])
	}
}

func TestASmallWindowIsListedInFullAsBefore(t *testing.T) {
	tree := scrolledList(600)
	text := tree.Text()
	if !strings.Contains(text, `"Big Sur" [out of view]`) || strings.Contains(text, "more that are scrolled") {
		t.Errorf("a three-control window was trimmed:\n%s", text)
	}
}
