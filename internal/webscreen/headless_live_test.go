//go:build integration

package webscreen

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/headless"
	"github.com/vedansh/vans/internal/uiact"
)

type tab struct {
	*headless.Chrome
	t   *testing.T
	ctx context.Context
}

func openHeadless(t *testing.T) *tab {
	t.Helper()
	c := headless.Open(t)
	return &tab{Chrome: c, t: t, ctx: c.Ctx}
}

func (h *tab) eval(script string) string { return h.Eval(script) }

func (h *tab) screen() *Screen {
	return &Screen{Page: h.Chrome, Browser: "Google Chrome", RealDrag: h.Drag, Log: h.t.Logf}
}

func servePages(t *testing.T) (string, string) { return headless.Serve(t, "testdata") }

func (h *tab) do(s *Screen, steps ...uiact.Step) {
	h.t.Helper()
	tree, err := s.Capture(h.ctx, "")
	if err != nil {
		h.t.Fatal(err)
	}
	if err := s.Act(h.ctx, uiact.Program{App: tree.App, Steps: steps}, tree); err != nil {
		h.t.Fatalf("%+v: %v", steps, err)
	}
}

func (h *tab) logSays(want string) {
	h.t.Helper()
	if got := h.eval("document.getElementById('log').textContent"); got != want {
		h.t.Errorf("the page says %q, want %q", got, want)
	}
}

func button(title string) uiact.Selector { return uiact.Selector{Role: "AXButton", Title: title} }

func TestHeadlessEveryKindOfStepWorksOnAPage(t *testing.T) {
	h := openHeadless(t)
	own, _ := servePages(t)
	if err := h.OpenInFront(h.ctx, own+"/controls.html"); err != nil {
		t.Fatal(err)
	}
	s := h.screen()

	t.Run("doubleclick", func(t *testing.T) {
		h.do(s, uiact.Step{Do: "doubleclick", Target: button("Quarterly report")})
		h.logSays("opened the row")
	})
	t.Run("rightclick", func(t *testing.T) {
		h.do(s, uiact.Step{Do: "rightclick", Target: button("Photo")})
		h.do(s, uiact.Step{Do: "click", Target: button("Rename")})
		h.logSays("renamed")
	})
	t.Run("hover", func(t *testing.T) {
		h.do(s, uiact.Step{Do: "hover", Target: button("Account")})
		h.do(s, uiact.Step{Do: "click", Target: button("Sign out")})
		h.logSays("signed out")
	})
	t.Run("set a range", func(t *testing.T) {
		h.do(s, uiact.Step{Do: "set", Target: uiact.Selector{Role: "AXSlider", Title: "Volume"}, Text: "70"})
		if got := h.eval("document.getElementById('volume').value"); got != "70" {
			t.Errorf("Volume is %s, want 70", got)
		}
	})
	t.Run("set a stepped slider", func(t *testing.T) {
		h.do(s, uiact.Step{Do: "set", Target: uiact.Selector{Role: "AXSlider", Title: "Brightness"}, Text: "80"})
		if got := h.eval("document.getElementById('brightness').getAttribute('aria-valuenow')"); got != "80" {
			t.Errorf("Brightness is %s, want 80", got)
		}
	})
	t.Run("a control scrolled out of its list", func(t *testing.T) {
		h.do(s, uiact.Step{Do: "click", Target: button("Load more")})
		h.logSays("loaded more")
	})
	t.Run("drag", func(t *testing.T) {
		to := button("Done column")
		h.do(s, uiact.Step{Do: "drag", Target: button("Card A"), To: &to})
		h.logSays("moved Card A to Done")
	})
}

func TestHeadlessPagesInsidePagesAreUsedWhereTheBrowserAllows(t *testing.T) {
	h := openHeadless(t)
	own, elsewhere := servePages(t)
	if err := h.OpenInFront(h.ctx, own+"/frames.html"); err != nil {
		t.Fatal(err)
	}
	s := h.screen()
	time.Sleep(500 * time.Millisecond)

	t.Run("a frame written inline", func(t *testing.T) {
		h.do(s,
			uiact.Step{Do: "type", Target: uiact.Selector{Role: "AXTextField", Title: "Coupon"}, Text: "SAVE10"},
			uiact.Step{Do: "click", Target: button("Apply coupon")},
		)
		h.logSays("applied SAVE10")
	})
	t.Run("a frame from the same site", func(t *testing.T) {
		h.do(s, uiact.Step{Do: "click", Target: button("Pay later")})
		h.logSays("paid later")
	})
	t.Run("a frame from another site", func(t *testing.T) {
		tree, err := s.Capture(h.ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if n := len(tree.Find(button("Pay later"))); n != 1 {
			t.Errorf("Pay later is listed %d times; the one from another site should not be", n)
		}
		host := strings.TrimPrefix(strings.Split(elsewhere, ":")[1], "//")
		walled := false
		for _, el := range tree.Elements {
			if el.Role == "AXGroup" && strings.Contains(el.Title, "cannot be read or used") && strings.Contains(el.Title, host) && !el.Enabled {
				walled = true
			}
		}
		if !walled {
			t.Errorf("the frame from %s is not named as one that cannot be used: %s", host, titles(tree))
		}
	})
}

func TestHeadlessTypingIsPutBackAsItWas(t *testing.T) {
	h := openHeadless(t)
	own, _ := servePages(t)
	if err := h.OpenInFront(h.ctx, own+"/controls.html"); err != nil {
		t.Fatal(err)
	}
	s := h.screen()
	name := uiact.Selector{Role: "AXTextField", Title: "Name"}

	tree, err := s.Capture(h.ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	was, ok := s.ValueOf(h.ctx, tree, name)
	if !ok || was != "Ada" {
		t.Fatalf("Name reads %q (%v), want Ada", was, ok)
	}
	h.do(s, uiact.Step{Do: "type", Target: name, Text: "Grace"})
	if got := h.eval("document.getElementById('name').value"); got != "Grace" {
		t.Fatalf("Name is %q after typing, want Grace", got)
	}
	if err := s.PutBack(h.ctx, "", name, was); err != nil {
		t.Fatal(err)
	}
	if got := h.eval("document.getElementById('name').value"); got != "Ada" {
		t.Errorf("Name is %q after undo, want Ada", got)
	}

	email := uiact.Selector{Role: "AXTextField", Title: "Email"}
	h.do(s, uiact.Step{Do: "type", Target: email, Text: "ada@example.com"})
	if err := s.PutBack(h.ctx, "", email, ""); err != nil {
		t.Fatal(err)
	}
	if got := h.eval("document.getElementById('email').value"); got != "" {
		t.Errorf("Email is %q after undo, want it empty as it was", got)
	}
}

func titles(t uiact.Tree) string {
	var out []string
	for _, el := range t.Elements {
		out = append(out, fmt.Sprintf("%s %q", el.Role, el.Title))
	}
	return strings.Join(out, ", ")
}
