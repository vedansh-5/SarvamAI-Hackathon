package engine

import (
	"context"
	"errors"
	"strings"

	"github.com/vedansh/vans/internal/answer"
	"github.com/vedansh/vans/internal/synth"
)

// Answerer replies to a question from rows already extracted. Implemented by
// the synthesis backend, and an interface here so a build without one simply
// shows the rows.
type Answerer interface {
	Answer(ctx context.Context, req synth.AnswerRequest) (answer.Answer, error)
}

// answerFor produces a reply, or nothing at all.
//
// A failure here never fails the task. The rows are already extracted and
// already worth showing: losing the whole result because a summary could not
// be written would be trading the thing that works for the thing on top of it.
func (e *Engine) answerFor(ctx context.Context, question string, plan *Plan) {
	if e.Answers == nil || len(plan.Rows) == 0 || strings.TrimSpace(question) == "" {
		return
	}

	req := synth.AnswerRequest{
		Question: question,
		Fields:   plan.Fields,
		Rows:     plan.Rows,
	}

	for attempt := 1; attempt <= e.maxSynthesis(); attempt++ {
		a, err := e.Answers.Answer(ctx, req)
		if err != nil {
			e.logf("no answer written: %v", err)
			return
		}

		if err := answer.Validate(a, plan.Rows); err != nil {
			var ungrounded *answer.UngroundedError
			if errors.As(err, &ungrounded) {
				e.logf("answer refused: %s", strings.Join(ungrounded.Reasons, "; "))
				req.Failures = ungrounded.Reasons
				continue
			}
			e.logf("no answer written: %v", err)
			return
		}

		plan.Answer = &a
		plan.AnswerCalls = attempt
		return
	}

	e.logf("no answer survived being checked against the results")
}
