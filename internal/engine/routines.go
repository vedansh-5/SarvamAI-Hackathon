package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vedansh/vans/internal/store"
)

type RoutineBook interface {
	SaveRoutine(ctx context.Context, name, instruction string) error
	Routine(ctx context.Context, name string) (string, bool, error)
	Routines(ctx context.Context) ([]store.Routine, error)
	ForgetRoutine(ctx context.Context, name string) (bool, error)
}

const quote = `["\x{201C}\x{201D}']?`

var (
	saveTheLast   = regexp.MustCompile(`(?i)^\s*save\s+(?:that|this|it|the last one)\s+as\s+(?:a\s+|the\s+|my\s+)?routine(?:\s+(?:called|named))?\s+` + quote + `(.+?)` + quote + `\s*$`)
	saveARoutine  = regexp.MustCompile(`(?i)^\s*(?:save|add|make)\s+(?:a\s+)?routine\s+` + quote + `(.+?)` + quote + `\s*[:=]\s*(.+)$`)
	listRoutines  = regexp.MustCompile(`(?i)^\s*(?:(?:show|list)(?:\s+me)?\s+)?(?:my\s+)?routines\s*\??\s*$`)
	forgetRoutine = regexp.MustCompile(`(?i)^\s*(?:forget|delete|remove)\s+(?:the\s+|my\s+)?routine\s+` + quote + `(.+?)` + quote + `\s*$`)
	runRoutine    = regexp.MustCompile(`(?i)^\s*(?:run|do|start)\s+(?:my\s+|the\s+)?(?:routine\s+)?` + quote + `(.+?)` + quote + `\s*$`)
)

func (e *Engine) aboutRoutines(ctx context.Context, instruction string) (*Plan, bool, error) {
	if e.Routines == nil {
		return nil, false, nil
	}

	if listRoutines.MatchString(instruction) {
		all, err := e.Routines.Routines(ctx)
		if err != nil {
			return nil, true, err
		}
		if len(all) == 0 {
			return saying("Routines", "None yet. After a task, say \u201Csave that as routine morning report\u201D, then just say \u201Cmorning report\u201D."), true, nil
		}
		var b strings.Builder
		for _, r := range all {
			fmt.Fprintf(&b, "\u2022 %s \u2014 %s\n", r.Name, r.Instruction)
		}
		b.WriteString("\nSay a routine\u2019s name to run it.")
		return saying("Routines", b.String()), true, nil
	}

	if m := forgetRoutine.FindStringSubmatch(instruction); m != nil {
		gone, err := e.Routines.ForgetRoutine(ctx, m[1])
		if err != nil {
			return nil, true, err
		}
		if !gone {
			return saying("No such routine", fmt.Sprintf("There is no routine called \u201C%s\u201D.", m[1])), true, nil
		}
		return saying("Routine removed", m[1]), true, nil
	}

	if m := saveARoutine.FindStringSubmatch(instruction); m != nil {
		return e.keepRoutine(ctx, m[1], m[2])
	}

	if m := saveTheLast.FindStringSubmatch(instruction); m != nil {
		if e.History == nil {
			return saying("Nothing to save", "VANS has no record of the last thing asked."), true, nil
		}
		recent, err := e.History.RecentInstructions(ctx)
		if err != nil {
			return nil, true, err
		}
		if len(recent) == 0 {
			return saying("Nothing to save", "Ask for something first, then save it."), true, nil
		}
		return e.keepRoutine(ctx, m[1], recent[0].Text)
	}
	return nil, false, nil
}

func (e *Engine) keepRoutine(ctx context.Context, name, instruction string) (*Plan, bool, error) {
	if err := e.Routines.SaveRoutine(ctx, name, instruction); err != nil {
		return nil, true, err
	}
	return saying("Routine saved", fmt.Sprintf("\u201C%s\u201D now means:\n\n%s\n\nSay its name to run it.",
		strings.TrimSpace(name), strings.TrimSpace(instruction))), true, nil
}

func (e *Engine) routineFor(ctx context.Context, instruction string) (string, bool) {
	if e.Routines == nil {
		return "", false
	}
	candidates := []string{instruction}
	if m := runRoutine.FindStringSubmatch(instruction); m != nil {
		candidates = append(candidates, m[1])
	}
	for _, name := range candidates {
		if saved, ok, err := e.Routines.Routine(ctx, name); err == nil && ok {
			return saved, true
		}
	}
	return "", false
}
