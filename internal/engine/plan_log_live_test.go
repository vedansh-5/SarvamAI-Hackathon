//go:build integration

package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/vedansh/vans/internal/synth"
)

type loggedPlans struct {
	t     *testing.T
	plans PlanWriter
}

func (l loggedPlans) WritePlan(ctx context.Context, req synth.PlanRequest) (synth.PlanAnswer, error) {
	answer, err := l.plans.WritePlan(ctx, req)
	steps, _ := json.Marshal(answer.Steps)
	l.t.Logf("plan asked (done %d, most %d): %d step(s) %s whole=%v done=%v stuck=%v why=%q err=%v",
		len(req.Done), req.Most, len(answer.Steps), steps, answer.Whole, answer.Done, answer.Stuck, answer.Why, err)
	return answer, err
}
