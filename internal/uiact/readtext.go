package uiact

import (
	"context"
	"strings"
)

const (
	LongestTextRead = 4000
	MostTextBytes   = 20000
)

type ScreenText struct {
	App    string
	Window string
	Lines  []string
}

type TextReader interface {
	ReadText(ctx context.Context, app string) (ScreenText, error)
}

var textWalker = walkAppText

func (t Tree) TextLines(most int) []string {
	var lines []string
	last := ""
	for _, el := range t.Elements {
		if InMenuBar(el) || role(el.Role) == "securetextfield" || t.outOfView(el) {
			continue
		}
		title, value := strings.TrimSpace(el.Title), strings.TrimSpace(el.Value)
		if strings.HasPrefix(strings.ToLower(title), unhelpfulFieldName) {
			title = ""
		}
		line := title
		switch {
		case title == "":
			line = value
		case value != "" && value != title && !toggles(el):
			line = title + ": " + value
		}
		line = strings.Join(strings.Fields(line), " ")
		if line == "" || line == last {
			continue
		}
		lines = append(lines, line)
		last = line
	}

	size := 0
	for i := len(lines) - 1; i >= 0; i-- {
		size += len(lines[i]) + 1
		if size > most {
			return lines[i+1:]
		}
	}
	return lines
}

func toggles(el Element) bool {
	_, ok := toggled(el)
	return ok
}

func (s System) ReadText(ctx context.Context, app string) (ScreenText, error) {
	if _, err := Capture(ctx, s.Runner, app); err != nil {
		return ScreenText{}, err
	}
	pid, err := pidFor(ctx, s.Runner, app)
	if err != nil {
		return ScreenText{}, err
	}
	tree, err := textWalker(pid, app)
	if err != nil {
		return ScreenText{}, err
	}
	return ScreenText{App: tree.App, Window: tree.Window, Lines: tree.TextLines(MostTextBytes)}, nil
}
