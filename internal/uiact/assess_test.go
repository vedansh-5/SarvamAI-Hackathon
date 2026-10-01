package uiact

import (
	"fmt"
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
