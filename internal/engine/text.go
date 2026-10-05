package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/synth"
)

type TextWorker interface {
	WorkOnText(ctx context.Context, req synth.TextRequest) (synth.TextAnswer, error)
}

const longestTextToWorkOn = 12000

func (e *Engine) canWorkOnText() bool { return e.Texts != nil }

func (e *Engine) workOnText(ctx context.Context, in Ask) (*Plan, error) {
	selected := strings.TrimSpace(e.selectionIn(ctx, in.Context))
	cut := false
	if runes := []rune(selected); len(runes) > longestTextToWorkOn {
		selected, cut = string(runes[:longestTextToWorkOn]), true
	}

	if selected == "" && refersToWhatIsInFront(in.Instruction) && !carriesItsOwnText(in.Instruction) {
		where := "the window in front"
		if in.Context != nil && strings.TrimSpace(in.Context.AppName) != "" {
			where = strings.TrimSpace(in.Context.AppName)
		}
		return saying("Select the text first",
			fmt.Sprintf("Nothing is selected in %s. Select the text you mean, then ask again.", where)), nil
	}

	given := selected
	if given == "" {
		given = textInTheRequest(in.Instruction)
	}

	answer, err := e.Texts.WorkOnText(ctx, synth.TextRequest{Ask: in.Instruction, Text: given})
	if err != nil {
		return nil, err
	}
	if answer.Cannot {
		return saying("Can’t do that with this text", reasonOr(answer.Why, "there was no text to work on")), nil
	}

	plan := saying(titleFor(in.Instruction), answer.Text)
	plan.Copy = answer.Text
	plan.SynthesisCalls = 1
	if cut {
		plan.Say += fmt.Sprintf("\n\n(Only the first %d characters of the selection were used.)", longestTextToWorkOn)
	}
	return plan, nil
}

func carriesItsOwnText(instruction string) bool {
	return strings.ContainsAny(instruction, "\"“”") || strings.Contains(instruction, ":")
}

var (
	quotedInTheRequest = regexp.MustCompile(`“([^”]+)”|"([^"]+)"`)
	afterAColon        = regexp.MustCompile(`(?s):\s+(\S.*)$`)
)

func textInTheRequest(instruction string) string {
	longest := ""
	for _, m := range quotedInTheRequest.FindAllStringSubmatch(instruction, -1) {
		for _, part := range m[1:] {
			if len([]rune(part)) > len([]rune(longest)) {
				longest = part
			}
		}
	}
	if longest = strings.TrimSpace(longest); longest != "" {
		return longest
	}
	if m := afterAColon.FindStringSubmatch(instruction); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func titleFor(instruction string) string {
	title := strings.TrimSpace(instruction)
	if runes := []rune(title); len(runes) > 60 {
		title = string(runes[:60]) + "…"
	}
	return title
}

func (e *Engine) selectedTextIsMeant(ctx context.Context, in Ask, kind route.Kind) bool {
	if kind != route.KindPageHere || !e.canWorkOnText() || !refersToWhatIsInFront(in.Instruction) {
		return false
	}
	return strings.TrimSpace(e.selectionIn(ctx, in.Context)) != ""
}
