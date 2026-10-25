package engine

import (
	"context"
	"fmt"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type ScreenReader interface {
	ReadScreen(ctx context.Context, req synth.ReadRequest) (synth.ReadAnswer, error)
}

func (e *Engine) canReadScreens() bool {
	if e.Reader == nil || e.UIScreen == nil {
		return false
	}
	_, ok := e.nativeScreen().(uiact.TextReader)
	return ok
}

func (e *Engine) readTheScreen(ctx context.Context, in Ask) (*Plan, error) {
	app, err := e.appForTask(ctx, in.Instruction, in.Context)
	if err != nil {
		return nil, err
	}
	if where, no := forbidden(e.rules(ctx), app); no {
		return nil, &NotAllowedError{Where: where, Rule: "you have told Vspk to leave it alone, and that includes reading what it shows"}
	}

	on, name := e, app
	if e.canDrivePages() && e.isPageBrowser(ctx, app) {
		on = e.onThePage()
	}
	reader, ok := on.UIScreen.(uiact.TextReader)
	if !ok {
		return nil, fmt.Errorf("engine: this build cannot read what %s shows", app)
	}
	shown, err := reader.ReadText(ctx, name)
	if err != nil {
		return nil, err
	}
	if where, no := forbidden(e.rules(ctx), shown.App); no {
		return nil, &NotAllowedError{Where: where, Rule: "you have told Vspk to leave it alone, and that includes reading what it shows"}
	}
	if len(shown.Lines) == 0 {
		return saying("Nothing to read", fmt.Sprintf("%s is showing no text Vspk can read.", app)), nil
	}

	e.logf("reading what %s shows (%d line(s)) to answer this", shown.App, len(shown.Lines))
	answer, err := e.Reader.ReadScreen(ctx, synth.ReadRequest{
		Question: in.Instruction, App: shown.App, Window: shown.Window, Lines: shown.Lines,
	})
	if err != nil {
		return nil, err
	}

	sent := fmt.Sprintf("\n\nRead from %s. The text it was showing was sent to the model to answer this.", shown.App)
	if answer.Cannot {
		plan := saying("Not on screen", reasonOr(answer.Why, "what is showing does not answer that")+sent)
		plan.SynthesisCalls = 1
		return plan, nil
	}
	plan := saying(titleFor(in.Instruction), answer.Text+sent)
	plan.Copy = answer.Text
	plan.SynthesisCalls = 1
	return plan, nil
}
