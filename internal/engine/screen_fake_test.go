package engine

import (
	"context"

	"github.com/vedansh/vans/internal/uiact"
)

type fakeScreen struct {
	tree     uiact.Tree
	captures int
	acts     int
	front    string
	typed    []string
	typeErr  error

	afterwards *uiact.Tree
}

func (f *fakeScreen) Capture(context.Context, string) (uiact.Tree, error) {
	f.captures++
	if f.acts > 0 && f.afterwards != nil {
		return *f.afterwards, nil
	}
	return f.tree, nil
}

func (f *fakeScreen) Act(_ context.Context, p uiact.Program, t uiact.Tree) error {
	f.acts++
	return uiact.Validate(p, t)
}

func (f *fakeScreen) Frontmost(context.Context) (string, error) {
	if f.front == "" {
		return "Editor", nil
	}
	return f.front, nil
}

func (f *fakeScreen) Type(_ context.Context, text string) error {
	f.typed = append(f.typed, text)
	return f.typeErr
}

func editorTree() uiact.Tree {
	return uiact.Tree{
		App:    "Editor",
		Window: "Editor",
		Elements: []uiact.Element{{
			Role: "AXTextArea", Title: "Message", Enabled: true,
			Position: []int{100, 200}, Size: []int{80, 40}, Path: []int{0, 1},
		}},
	}
}
