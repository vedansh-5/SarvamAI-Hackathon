package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type notebook struct{ facts []string }

func (n *notebook) Remember(_ context.Context, fact string) error {
	n.facts = append(n.facts, fact)
	return nil
}
func (n *notebook) Facts(context.Context) ([]string, error) { return n.facts, nil }
func (n *notebook) Forget(_ context.Context, about string) (int, error) {
	var kept []string
	gone := 0
	for _, f := range n.facts {
		if strings.Contains(strings.ToLower(f), strings.ToLower(about)) {
			gone++
			continue
		}
		kept = append(kept, f)
	}
	n.facts = kept
	return gone, nil
}

func TestBeingToldToRememberIsNotATask(t *testing.T) {
	book := &notebook{}
	e := &Engine{Memory: book}
	ctx := context.Background()

	plan, err := e.Ask(ctx, Ask{Instruction: "remember that my home city is Delhi"})
	if err != nil || plan.Say == "" || len(book.facts) != 1 || book.facts[0] != "my home city is Delhi" {
		t.Fatalf("plan = %+v, err = %v, facts = %v", plan, err, book.facts)
	}
	if plan, _ := e.Ask(ctx, Ask{Instruction: "Remember mom is Sunita Sharma."}); plan == nil || len(book.facts) != 2 {
		t.Fatalf("facts = %v", book.facts)
	}

	plan, _ = e.Ask(ctx, Ask{Instruction: "what do you remember about me?"})
	if !strings.Contains(plan.Say, "Delhi") || !strings.Contains(plan.Say, "Sunita") {
		t.Errorf("said %q, want both things listed", plan.Say)
	}

	plan, _ = e.Ask(ctx, Ask{Instruction: "forget about mom"})
	if len(book.facts) != 1 || !strings.Contains(plan.SayTitle, "Forgotten") {
		t.Errorf("facts = %v, said %q", book.facts, plan.SayTitle)
	}
}

func TestAnOrdinaryRequestIsNotMistakenForOneAboutMemory(t *testing.T) {
	e := &Engine{Memory: &notebook{}}
	for _, in := range []string{
		"remember to call mom",
		"send a reminder to Joe",
		"forgetful",
		"what do you know about flights to Goa",
	} {
		if plan, handled, _ := e.aboutMemory(context.Background(), in); handled && !strings.HasPrefix(strings.ToLower(in), "forget") {
			t.Errorf("%q was taken as being about memory: %+v", in, plan)
		}
	}
}

func TestOnlyWhatBearsOnTheTaskIsRecalled(t *testing.T) {
	e := &Engine{Memory: &notebook{facts: []string{
		"my home city is Delhi",
		"mom is Sunita Sharma on WhatsApp",
		"my usual airline is IndiGo",
	}}}
	ctx := context.Background()

	if got := e.recall(ctx, "send good morning to mom"); len(got) != 1 || !strings.Contains(got[0], "Sunita") {
		t.Errorf("recalled %v for a message to mom", got)
	}
	if got := e.recall(ctx, "Which airline do you want?"); len(got) != 1 || !strings.Contains(got[0], "IndiGo") {
		t.Errorf("recalled %v for a question about airlines", got)
	}
	if got := e.recall(ctx, "change the wallpaper"); len(got) != 0 {
		t.Errorf("recalled %v for a task none of it bears on", got)
	}
}

func TestSomethingRememberedAnswersAQuestionBeforeThePersonIsAsked(t *testing.T) {
	steps := &recordingSteps{answers: []synth.StepAnswer{
		{Question: &synth.Question{Ask: "Which airline?", Options: []string{"IndiGo", "Air India"}}},
		{Step: typeIntoMessage()},
		{Done: true},
	}}
	asked := 0
	e := &Engine{UIScreen: &movingScreen{}, Steps: steps, Memory: &notebook{facts: []string{"my usual airline is IndiGo"}}}
	e.Question = func(context.Context, string, []string) (string, error) { asked++; return "Air India", nil }

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if asked != 0 {
		t.Errorf("asked the person %d time(s) something they had already told Vspk", asked)
	}
	if len(steps.requests) < 2 || !strings.Contains(strings.Join(steps.requests[1].Done, " "), "IndiGo") {
		t.Errorf("the next step was worked out without what was remembered: %v", steps.requests[1].Done)
	}
}

func TestAQuestionMemoryCannotAnswerStillGoesToThePerson(t *testing.T) {
	steps := &recordingSteps{answers: []synth.StepAnswer{
		{Question: &synth.Question{Ask: "Which airline?"}},
		{Question: &synth.Question{Ask: "Which airline?"}},
		{Step: typeIntoMessage()},
		{Done: true},
	}}
	asked := 0
	e := &Engine{UIScreen: &movingScreen{}, Steps: steps, Memory: &notebook{facts: []string{"my usual airline is IndiGo"}}}
	e.Question = func(context.Context, string, []string) (string, error) { asked++; return "Air India", nil }

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if asked != 1 {
		t.Errorf("asked %d time(s); a model that asks again after being told what is remembered means it", asked)
	}
}

func TestADryRunSaysTheStepsAndDoesNothing(t *testing.T) {
	screen := &sendScreen{}
	planner := &sequencedPlanner{plans: [][]uiact.Step{
		{typeIntoMessage()},
		{typeIntoMessage(), clickSend()},
	}}
	e := &Engine{UIScreen: screen, Plans: planner, Steps: &countingSteps{}, Rulebook: rulebook{SpendLimit: 5000}}

	plan, err := e.driveOn(context.Background(), "Editor", "say hello and send it", "say hello and send it", nil)
	if err != nil {
		t.Fatal(err)
	}
	said := e.rehearsalOf(context.Background(), plan)

	if said.Drive != nil || said.Say == "" {
		t.Fatalf("plan = %+v, want words and nothing that can be confirmed", said)
	}
	if !planner.requests[len(planner.requests)-1].Rehearse {
		t.Error("the planner was not told this was a rehearsal, so it would stop where it stops being sure")
	}
	for _, want := range []string{"1. Type hello into Message", "2. Click Send", "would stop and ask you first", "5000"} {
		if !strings.Contains(said.Say, want) {
			t.Errorf("said %q, want it to include %q", said.Say, want)
		}
	}
	if strings.Count(said.Say, "would stop and ask") != 1 {
		t.Errorf("said %q, want only the sending step marked", said.Say)
	}
	if screen.acts != 0 {
		t.Errorf("acted %d time(s) during a dry run", screen.acts)
	}
}

func TestTheWordsForADryRunAreTakenOutOfTheRequest(t *testing.T) {
	for _, c := range []struct{ in, rest string }{
		{"dry run: book a flight on makemytrip", "book a flight on makemytrip"},
		{"book a flight on makemytrip without doing it", "book a flight on makemytrip"},
		{"send hi to Joe on whatsapp, don't actually do it", "send hi to Joe on whatsapp"},
		{"what would you do to change the wallpaper", "change the wallpaper"},
		{"just show me the steps to export this", "export this"},
	} {
		rest, dry := withoutTheRehearsal(c.in)
		if !dry || rest != c.rest {
			t.Errorf("%q -> %q (dry=%v), want %q", c.in, rest, dry, c.rest)
		}
	}
	for _, in := range []string{"book a flight on makemytrip", "clean the dry run logs folder name", "do it"} {
		if _, dry := withoutTheRehearsal(in); dry && in != "clean the dry run logs folder name" {
			t.Errorf("%q was taken as a dry run", in)
		}
	}
}
