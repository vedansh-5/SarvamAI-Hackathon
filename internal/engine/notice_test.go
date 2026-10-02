package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/uiact"
)

func noticeTree(buttons ...string) uiact.Tree {
	tree := editorTree()
	for i, name := range buttons {
		tree.Elements = append(tree.Elements, uiact.Element{
			Role: "AXButton", Title: name, Enabled: true, Notice: true,
			Position: []int{100 + 120*i, 600}, Size: []int{110, 36}, Path: []int{1, i},
		})
	}
	return tree
}

func TestACookieNoticeIsClosedByItsNoAndNeverItsYes(t *testing.T) {
	for _, tc := range []struct {
		buttons []string
		want    string
	}{
		{[]string{"Accept all", "Reject all"}, "Reject all"},
		{[]string{"Accept All Cookies", "Use necessary cookies only"}, "Use necessary cookies only"},
		{[]string{"Got it", "Decline"}, "Decline"},
		{[]string{"Accept all", "Manage preferences"}, ""},
		{[]string{"OK"}, ""},
	} {
		el, ok := wayToSayNo(noticeTree(tc.buttons...))
		if tc.want == "" {
			if ok {
				t.Errorf("%v: chose %q, want nothing clicked when there is no way to say no", tc.buttons, el.Title)
			}
			continue
		}
		if !ok || el.Title != tc.want {
			t.Errorf("%v: chose %q, want %q", tc.buttons, el.Title, tc.want)
		}
	}
}

func TestAButtonOutsideANoticeIsNeverTakenForOne(t *testing.T) {
	tree := editorTree()
	tree.Elements = append(tree.Elements, uiact.Element{
		Role: "AXButton", Title: "Decline", Enabled: true,
		Position: []int{100, 600}, Size: []int{110, 36}, Path: []int{1, 0},
	})
	if el, ok := wayToSayNo(tree); ok {
		t.Errorf("clicked %q, which is a button of the page and not of a cookie notice", el.Title)
	}
}

func TestClosingTheNoticeCostsNoModelCallAndIsToldToTheModel(t *testing.T) {
	look := lookBeforeActing
	lookBeforeActing = 0
	t.Cleanup(func() { lookBeforeActing = look })

	screen := &fakeScreen{tree: noticeTree("Accept all", "Reject all")}
	steps := &countingSteps{}
	e := &Engine{UIScreen: screen, Steps: steps}

	told, closed := e.closeTheNotice(context.Background(), "Editor")
	if !closed || screen.acts != 1 {
		t.Fatalf("closed = %v after %d act(s), want the one click", closed, screen.acts)
	}
	if steps.calls != 0 {
		t.Errorf("asked the model %d time(s) to close a cookie notice", steps.calls)
	}
	if !strings.Contains(told, "Reject all") {
		t.Errorf("told = %q, want it to say what was clicked", told)
	}
}
