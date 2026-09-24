package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
)

// Every way a coached run can end short has to be tellable from VANS breaking,
// or the panel says "something went wrong inside VANS" about a window that
// simply did not have what the step needed.
func TestAStoppedRunIsNotAFault(t *testing.T) {
	quickGuide(t)

	for _, c := range []struct {
		name string
		run  func() error
	}{
		{"nobody acted", func() error {
			e := &Engine{
				UIScreen: &fakeScreen{tree: editorTree()},
				Steps:    &alwaysStep{step: typeIntoMessage()},
			}
			_, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil)
			return err
		}},
		{"nothing to point at", func() error {
			e := &Engine{
				UIScreen: &fakeScreen{tree: editorTree()},
				Steps:    &scriptedSteps{answers: []synth.StepAnswer{{Done: true, Why: "it is in another application"}}},
			}
			_, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil)
			return err
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := c.run()
			if err == nil {
				t.Fatal("want an error")
			}
			if !errors.Is(err, ErrCoachStopped) {
				t.Errorf("err = %v, want it marked as the steps stopping rather than a fault", err)
			}
			if WhereItStopped(err) != "Editor" {
				t.Errorf("WhereItStopped = %q, want Editor so the panel can say where", WhereItStopped(err))
			}
			// The sentence is the explanation and must survive being marked.
			if strings.TrimSpace(err.Error()) == "" {
				t.Error("the reason was lost")
			}
		})
	}
}

// A real fault still reads as one.
func TestAMissingPieceIsStillAFault(t *testing.T) {
	e := &Engine{}
	_, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if errors.Is(err, ErrCoachStopped) {
		t.Errorf("a build that cannot coach at all was reported as the steps stopping: %v", err)
	}
}
