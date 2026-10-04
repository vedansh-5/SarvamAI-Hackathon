package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

type routineBook struct{ saved map[string]string }

func (r *routineBook) SaveRoutine(_ context.Context, name, instruction string) error {
	if r.saved == nil {
		r.saved = map[string]string{}
	}
	r.saved[strings.ToLower(name)] = instruction
	return nil
}
func (r *routineBook) Routine(_ context.Context, name string) (string, bool, error) {
	v, ok := r.saved[strings.ToLower(strings.TrimSpace(name))]
	return v, ok, nil
}
func (r *routineBook) Routines(context.Context) ([]store.Routine, error) {
	var out []store.Routine
	for k, v := range r.saved {
		out = append(out, store.Routine{Name: k, Instruction: v})
	}
	return out, nil
}
func (r *routineBook) ForgetRoutine(_ context.Context, name string) (bool, error) {
	_, ok := r.saved[strings.ToLower(name)]
	delete(r.saved, strings.ToLower(name))
	return ok, nil
}

type lastAsked struct{ text string }

func (l lastAsked) RecordInstruction(context.Context, string, string) error { return nil }
func (l lastAsked) FinishInstruction(context.Context, string, int) error    { return nil }
func (l lastAsked) ForgetInstruction(context.Context, string) error         { return nil }
func (l lastAsked) RecentInstructions(context.Context) ([]store.Remembered, error) {
	if l.text == "" {
		return nil, nil
	}
	return []store.Remembered{{Text: l.text}}, nil
}

func TestATaskCanBeGivenANameAndAskedForByIt(t *testing.T) {
	book := &routineBook{}
	e := &Engine{Routines: book, History: lastAsked{text: `send "good morning" to mom on whatsapp`}}
	ctx := context.Background()

	plan, handled, err := e.aboutRoutines(ctx, `save that as a routine called "morning hello"`)
	if err != nil || !handled || !strings.Contains(plan.Say, "good morning") {
		t.Fatalf("plan = %+v, handled = %v, err = %v", plan, handled, err)
	}

	for _, said := range []string{"morning hello", "Morning Hello", "run morning hello", "do my routine morning hello"} {
		got, ok := e.routineFor(ctx, said)
		if !ok || got != `send "good morning" to mom on whatsapp` {
			t.Errorf("%q -> %q (found=%v), want the saved task", said, got, ok)
		}
	}
	if _, ok := e.routineFor(ctx, "morning"); ok {
		t.Error("part of a routine's name ran it")
	}

	if _, handled, _ := e.aboutRoutines(ctx, "save routine weekly: open the report in Numbers"); !handled || book.saved["weekly"] == "" {
		t.Errorf("saved = %v, want a routine said in full kept too", book.saved)
	}
	if plan, _, _ := e.aboutRoutines(ctx, "routines"); !strings.Contains(plan.Say, "weekly") {
		t.Errorf("list = %q", plan.Say)
	}
	if _, _, _ = e.aboutRoutines(ctx, "forget routine weekly"); book.saved["weekly"] != "" {
		t.Error("the routine was not removed")
	}
}

func TestOrdinaryRequestsAreNotTakenForRoutineCommands(t *testing.T) {
	e := &Engine{Routines: &routineBook{}, History: lastAsked{text: "x"}}
	for _, in := range []string{
		"save this file as report.pdf in Preview", "save it as draft", "run the tests",
		"call mom on FaceTime", "call it a day", "show me my routine for Monday in Notes",
	} {
		if plan, handled, _ := e.aboutRoutines(context.Background(), in); handled {
			t.Errorf("%q was taken as a routine command: %+v", in, plan)
		}
	}
}

func TestALongRunStopsToCheckInEverySoManySteps(t *testing.T) {
	quickLook(t, 0)
	was := checkpointEvery
	checkpointEvery = 2
	t.Cleanup(func() { checkpointEvery = was })

	var answers []synth.StepAnswer
	for i := 0; i < 5; i++ {
		answers = append(answers, synth.StepAnswer{Step: clickMessage()})
	}
	answers = append(answers, synth.StepAnswer{Done: true})

	var paused []GuideStep
	resume := make(chan struct{}, 1)
	e := &Engine{UIScreen: &movingScreen{}, Steps: &scriptedSteps{answers: answers}}
	e.ShowStep = func(GuideStep) {}
	e.Resume = resume
	e.Paused = func(g GuideStep) {
		paused = append(paused, g)
		go func() {
			time.Sleep(5 * time.Millisecond)
			resume <- struct{}{}
		}()
	}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(paused) != 2 || paused[0].Number != 3 || paused[1].Number != 5 {
		t.Fatalf("checked in at %+v, want before steps 3 and 5", paused)
	}
	if !strings.Contains(paused[0].Ask, "2 steps done") {
		t.Errorf("said %q, want how far it has got", paused[0].Ask)
	}
}
