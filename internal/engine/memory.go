package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type Memory interface {
	Remember(ctx context.Context, fact string) error
	Facts(ctx context.Context) ([]string, error)
	Forget(ctx context.Context, about string) (int, error)
}

var (
	rememberThis = regexp.MustCompile(`(?i)^\s*(?:please\s+)?remember\s+(?:that\s+)?((?:my|i|i'm|im|our|we)\b.+|.+\b(?:is|are|means)\b.+)$`)
	forgetThis   = regexp.MustCompile(`(?i)^\s*(?:please\s+)?forget\s+(?:that\s+|about\s+|what you (?:know|remember) about\s+)?(.+)$`)
	whatIsKnown  = regexp.MustCompile(`(?i)^\s*(?:what do you (?:remember|know)(?: about me)?|(?:show|list|tell)(?: me)? what you (?:remember|know)(?: about me)?)\s*\??\s*$`)
)

const mostRecalled = 5

func (e *Engine) aboutMemory(ctx context.Context, instruction string) (*Plan, bool, error) {
	if e.Memory == nil {
		return nil, false, nil
	}

	if whatIsKnown.MatchString(instruction) {
		facts, err := e.Memory.Facts(ctx)
		if err != nil {
			return nil, true, err
		}
		if len(facts) == 0 {
			return saying("What Vspk remembers", "Nothing yet. Say \u201Cremember that my home city is Delhi\u201D and it will."), true, nil
		}
		return saying("What Vspk remembers", "\u2022 "+strings.Join(facts, "\n\u2022 ")+
			"\n\nSay \u201Cforget \u2026\u201D to remove one."), true, nil
	}

	if m := rememberThis.FindStringSubmatch(instruction); m != nil {
		fact := strings.TrimRight(strings.TrimSpace(m[1]), ".")
		if err := e.Memory.Remember(ctx, fact); err != nil {
			return nil, true, err
		}
		return saying("Remembered", fact+"\n\nVANS uses this when a task needs it, and only then."), true, nil
	}

	if m := forgetThis.FindStringSubmatch(instruction); m != nil {
		about := strings.TrimRight(strings.TrimSpace(m[1]), ".")
		gone, err := e.Memory.Forget(ctx, about)
		if err != nil {
			return nil, true, err
		}
		if gone == 0 {
			return saying("Nothing to forget", fmt.Sprintf("Vspk remembers nothing about \u201C%s\u201D.", about)), true, nil
		}
		return saying("Forgotten", fmt.Sprintf("%d thing(s) about \u201C%s\u201D.", gone, about)), true, nil
	}
	return nil, false, nil
}

func saying(title, text string) *Plan {
	return &Plan{Say: text, SayTitle: title}
}

var notWorthMatching = map[string]bool{
	"the": true, "and": true, "for": true, "that": true, "this": true, "with": true, "from": true,
	"are": true, "was": true, "you": true, "your": true, "our": true, "has": true, "have": true,
	"which": true, "what": true, "who": true, "how": true, "does": true, "can": true, "will": true,
	"use": true, "usual": true, "usually": true, "always": true, "prefer": true, "like": true,
}

func wordsOf(text string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(w) >= 3 && !notWorthMatching[w] {
			out = append(out, w)
		}
	}
	return out
}

func alike(a, b string) bool {
	if a == b {
		return true
	}
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	return len(short) >= 4 && strings.HasPrefix(long, short)
}

func (e *Engine) recall(ctx context.Context, about string) []string {
	if e.Memory == nil {
		return nil
	}
	facts, err := e.Memory.Facts(ctx)
	if err != nil || len(facts) == 0 {
		return nil
	}
	asked := wordsOf(about)

	var out []string
	for _, fact := range facts {
		matched := false
		for _, fw := range wordsOf(fact) {
			for _, aw := range asked {
				if alike(fw, aw) {
					matched = true
				}
			}
		}
		if matched {
			out = append(out, fact)
			if len(out) == mostRecalled {
				break
			}
		}
	}
	return out
}

func rememberedNote(facts []string) string {
	if len(facts) == 0 {
		return ""
	}
	return "The person has told Vspk to remember: " + strings.Join(facts, "; ") + ". Use it where the task depends on it."
}
