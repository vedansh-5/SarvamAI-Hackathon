package engine

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/vedansh/vans/internal/store"
)

type keptRuns interface {
	DriveTracesIn(ctx context.Context, app string) ([]store.KeptRun, error)
}

const (
	mostSimilarRuns  = 2
	mostSimilarSteps = 10
	leastAlike       = 0.5
	valueOfTheirs    = "…"
)

var (
	placeholder  = regexp.MustCompile(`\{[a-z]+\d+\}`)
	wordsOfATask = regexp.MustCompile(`[\p{L}\p{N}]+`)
	sayNothing   = map[string]bool{
		"the": true, "and": true, "for": true, "from": true, "with": true, "into": true, "onto": true,
		"this": true, "that": true, "how": true, "show": true, "please": true, "can": true, "you": true,
		"what": true, "where": true, "are": true, "does": true, "here": true, "then": true, "want": true,
	}
)

func tellingWords(text, app string) map[string]bool {
	text = placeholder.ReplaceAllString(strings.ToLower(text), " ")
	inTheName := map[string]bool{}
	for _, w := range wordsOfATask.FindAllString(strings.ToLower(app), -1) {
		inTheName[w] = true
	}
	out := map[string]bool{}
	for _, w := range wordsOfATask.FindAllString(text, -1) {
		if len([]rune(w)) < 3 || sayNothing[w] || inTheName[w] {
			continue
		}
		out[w] = true
	}
	return out
}

func overlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for w := range a {
		if b[w] {
			shared++
		}
	}
	return float64(shared) / float64(min(len(a), len(b)))
}

func plainShape(shape string) string {
	return strings.TrimSpace(strings.TrimPrefix(shape, "coach: "))
}

func howItWasDone(run store.KeptRun) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%q was done with:", placeholder.ReplaceAllString(plainShape(run.Shape), valueOfTheirs))
	for i, s := range run.Program.Steps {
		if i == mostSimilarSteps {
			b.WriteString("\n      (and more)")
			break
		}
		target := fmt.Sprintf("%s %q", s.Target.Role, placeholder.ReplaceAllString(s.Target.Title, valueOfTheirs))
		switch strings.ToLower(strings.TrimSpace(s.Do)) {
		case "type":
			fmt.Fprintf(&b, "\n      %d. type %q into %s", i+1, placeholder.ReplaceAllString(s.Text, valueOfTheirs), target)
		case "press":
			fmt.Fprintf(&b, "\n      %d. press %s in %s", i+1, s.Chord(), target)
		default:
			fmt.Fprintf(&b, "\n      %d. %s %s", i+1, strings.ToLower(strings.TrimSpace(s.Do)), target)
		}
	}
	return b.String()
}

func (e *Engine) similarRuns(ctx context.Context, app, task string) []string {
	kept, ok := e.Traces.(keptRuns)
	if !ok || strings.TrimSpace(app) == "" {
		return nil
	}
	runs, err := kept.DriveTracesIn(ctx, app)
	if err != nil || len(runs) == 0 {
		return nil
	}

	asked := tellingWords(task, app)
	type scored struct {
		run   store.KeptRun
		score float64
	}
	var near []scored
	seen := map[string]bool{}
	for _, run := range runs {
		shape := plainShape(run.Shape)
		if seen[shape] {
			continue
		}
		seen[shape] = true
		if score := overlap(asked, tellingWords(shape, app)); score >= leastAlike {
			near = append(near, scored{run, score})
		}
	}
	sort.SliceStable(near, func(i, j int) bool { return near[i].score > near[j].score })

	var out []string
	for _, n := range near {
		if len(out) == mostSimilarRuns {
			break
		}
		out = append(out, howItWasDone(n.run))
	}
	return out
}

type askedKey struct{}

func withAsked(ctx context.Context, asked string) context.Context {
	if strings.TrimSpace(asked) == "" {
		return ctx
	}
	return context.WithValue(ctx, askedKey{}, asked)
}

func askedIn(ctx context.Context, fallback string) string {
	if asked, _ := ctx.Value(askedKey{}).(string); asked != "" {
		return asked
	}
	return fallback
}
