package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/when"
)

type ScheduleBook interface {
	AddSchedule(ctx context.Context, what, rule string, next time.Time) (int64, error)
	Schedules(ctx context.Context) ([]store.Schedule, error)
	DueSchedules(ctx context.Context, now time.Time) ([]store.Schedule, error)
	MoveSchedule(ctx context.Context, id int64, next time.Time) error
	DropSchedule(ctx context.Context, id int64) error
	CancelSchedules(ctx context.Context, what string) (int, error)
}

var (
	listSchedules  = regexp.MustCompile(`(?i)^\s*(?:(?:(?:show|list)(?:\s+me)?\s+)?(?:my\s+)?schedules?|what(?:'s| is) scheduled)\s*\??\s*$`)
	cancelSchedule = regexp.MustCompile(`(?i)^\s*(?:unschedule|(?:cancel|remove|delete|stop)\s+(?:the\s+|my\s+)?schedule(?:\s+for)?)\s+` + quote + `(.+?)` + quote + `\s*$`)
	asksToSchedule = regexp.MustCompile(`(?i)^\s*(?:schedule\b|every\s+\S+|(?:today|tomorrow)\s+at\b|at\s+\d|at\s+(?:noon|midnight)\b|in\s+(?:\d+|an?)\s*(?:min|hour|hr))`)
	leadIn         = regexp.MustCompile(`(?i)^\s*(?:schedule\b)?[\s,:\x{2014}-]*(?:to\s+)?(?:run|do|start)?[\s,:\x{2014}-]*`)
)

var clock = time.Now

func whatToDo(rest string) string {
	rest = leadIn.ReplaceAllString(rest, "")
	rest = strings.TrimSpace(strings.Trim(strings.TrimSpace(rest), ",:;"))
	return strings.Trim(rest, "\"“”'")
}

func (e *Engine) aboutSchedules(ctx context.Context, instruction string) (*Plan, bool, error) {
	if e.Schedules == nil {
		return nil, false, nil
	}

	if listSchedules.MatchString(instruction) {
		all, err := e.Schedules.Schedules(ctx)
		if err != nil {
			return nil, true, err
		}
		if len(all) == 0 {
			return saying("Schedules", "Nothing is scheduled. Say “every weekday at 9am run morning report”, or “in 20 minutes remind me to stretch”."), true, nil
		}
		var b strings.Builder
		for _, sch := range all {
			fmt.Fprintf(&b, "• %s — %s (next %s)\n", sch.What, ruleOf(sch).String(), sch.Next.Format("Mon 2 Jan, 3:04 pm"))
		}
		b.WriteString("\nSay “cancel schedule …” to remove one.")
		return saying("Schedules", b.String()), true, nil
	}

	if m := cancelSchedule.FindStringSubmatch(instruction); m != nil {
		gone, err := e.Schedules.CancelSchedules(ctx, m[1])
		if err != nil {
			return nil, true, err
		}
		if gone == 0 {
			return saying("Nothing to cancel", fmt.Sprintf("Nothing scheduled matches “%s”.", m[1])), true, nil
		}
		return saying("Schedule cancelled", fmt.Sprintf("%d removed for “%s”.", gone, m[1])), true, nil
	}

	if !asksToSchedule.MatchString(instruction) {
		return nil, false, nil
	}
	rule, rest, ok := when.Parse(instruction, clock())
	if !ok {
		return nil, false, nil
	}
	what := whatToDo(rest)
	if what == "" {
		return saying("Schedule what?", "Say what to do at that time, like “every weekday at 9am run morning report”."), true, nil
	}
	next, ok := rule.Next(clock())
	if !ok {
		return saying("That time has gone", "Say a time that is still to come."), true, nil
	}
	encoded, err := json.Marshal(rule)
	if err != nil {
		return nil, true, err
	}
	if _, err := e.Schedules.AddSchedule(ctx, what, string(encoded), next); err != nil {
		return nil, true, err
	}
	return saying("Scheduled", fmt.Sprintf(
		"“%s”, %s.\n\nNext: %s. VANS will bring it up then and wait for you to say go; it does not start anything on its own.\n\nIt needs VANS to be running at the time.",
		what, rule.String(), next.Format("Mon 2 Jan, 3:04 pm"))), true, nil
}

func ruleOf(sch store.Schedule) when.Rule {
	var rule when.Rule
	_ = json.Unmarshal([]byte(sch.Rule), &rule)
	return rule
}

type Due struct {
	What string
	When string
}

func (e *Engine) DueNow(ctx context.Context, now time.Time) []Due {
	if e.Schedules == nil {
		return nil
	}
	due, err := e.Schedules.DueSchedules(ctx, now)
	if err != nil {
		e.logf("could not read what is scheduled: %v", err)
		return nil
	}

	var out []Due
	for _, sch := range due {
		rule := ruleOf(sch)
		next, again := rule.Next(now)
		if again {
			err = e.Schedules.MoveSchedule(ctx, sch.ID, next)
		} else {
			err = e.Schedules.DropSchedule(ctx, sch.ID)
		}
		if err != nil {
			e.logf("could not move %q on to its next time, so it is not brought up: %v", sch.What, err)
			continue
		}
		out = append(out, Due{What: sch.What, When: rule.String()})
	}
	return out
}
