package uiact

import "testing"

func TestTypingIntoARowIsOnlyAllowedWhenPointing(t *testing.T) {
	tree := Tree{App: "Postman", Elements: []Element{{
		Role: "AXRow", Title: "Variable", Enabled: true,
		Position: []int{540, 222}, Size: []int{942, 28}, Path: []int{0, 1},
	}}}
	p := Program{App: "Postman", Steps: []Step{{
		Do: "type", Target: Selector{Role: "AXRow", Title: "Variable"}, Text: "baseUrl",
	}}}

	if Validate(p, tree) == nil {
		t.Error("typing into a row was allowed for doing it, where nothing can type into it")
	}
	if err := ValidateToPoint(p, tree); err != nil {
		t.Errorf("pointing at a row to type in it was refused: %v", err)
	}
}
