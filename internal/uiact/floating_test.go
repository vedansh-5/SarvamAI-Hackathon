package uiact

import "testing"

func popUpTree() Tree {
	return Tree{App: "Editor", Elements: []Element{
		{Role: "AXPopUpButton", Title: "Colour", Enabled: true, Position: []int{100, 100}, Size: []int{160, 30}, Path: []int{3}},
		{Role: "AXMenu", Enabled: true, Position: []int{100, 96}, Size: []int{160, 80}, Path: []int{3, 0}},
		{Role: "AXMenuItem", Title: "Red", Enabled: true, Position: []int{100, 100}, Size: []int{160, 22}, Path: []int{3, 0, 0}},
		{Role: "AXMenuItem", Title: "Blue", Enabled: true, Position: []int{100, 146}, Size: []int{160, 22}, Path: []int{3, 0, 2}},
		{Role: "AXScrollArea", Enabled: true, Position: []int{100, 300}, Size: []int{200, 60}, Path: []int{4}},
		{Role: "AXRow", Title: "Gamma", Enabled: true, Position: []int{100, 300}, Size: []int{200, 24}, Path: []int{4, 0}},
		{Role: "AXCell", Title: "Gamma", Enabled: true, Position: []int{106, 300}, Size: []int{180, 24}, Path: []int{4, 0, 0}},
		{Role: "AXRow", Title: "Delta", Enabled: true, Position: []int{100, 400}, Size: []int{200, 24}, Path: []int{4, 1}},
	}}
}

func TestAnItemInAnOpenMenuIsNotHiddenByTheButtonThatOpenedIt(t *testing.T) {
	was := displays
	displays = func() (box, bool) { return box{0, 0, 1700, 1100}, true }
	t.Cleanup(func() { displays = was })

	tree := popUpTree()
	if tree.OutOfView(Selector{Role: "AXMenuItem", Title: "Blue"}) {
		t.Error("Blue hangs below its pop-up button and was called out of view")
	}
	if !tree.OutOfView(Selector{Role: "AXRow", Title: "Delta"}) {
		t.Error("a row below its scroll area was called in view; only menus float")
	}
}

func TestWhatIsUnderThePointerMayBeAPartOfTheTarget(t *testing.T) {
	tree := popUpTree()
	row := tree.Elements[5]

	if !tree.partOf(row, "AXCell", "", 200, 312) {
		t.Error("the cell inside the row was not accepted as the row")
	}
	if tree.partOf(row, "AXButton", "Delete", 200, 312) {
		t.Error("a button that is no part of the row was accepted as the row")
	}
	if tree.partOf(row, "AXCell", "", 200, 500) {
		t.Error("a cell somewhere else was accepted as the row")
	}
}
