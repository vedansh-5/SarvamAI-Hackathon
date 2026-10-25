package synth

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/vedansh/vans/internal/answer"
	"github.com/vedansh/vans/internal/appact"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/uiact"
	"github.com/vedansh/vans/internal/webact"
)

type Model interface {
	Synthesize(ctx context.Context, req Request) (extract.Program, error)
	Decide(ctx context.Context, req route.Request) (route.Decision, error)
	Answer(ctx context.Context, req AnswerRequest) (answer.Answer, error)
	WriteAction(ctx context.Context, req AppActRequest) (appact.Program, error)
	WriteWebAction(ctx context.Context, req WebActRequest) (webact.Program, error)
	WriteUIAction(ctx context.Context, req UIActRequest) (uiact.Program, error)
	WriteStep(ctx context.Context, req StepRequest) (StepAnswer, error)
	WritePlan(ctx context.Context, req PlanRequest) (PlanAnswer, error)
	WriteStyle(ctx context.Context, req StyleRequest) (StyleAnswer, error)
	WhichApp(ctx context.Context, req AppChoiceRequest) (AppChoice, error)
	WorkOnText(ctx context.Context, req TextRequest) (TextAnswer, error)
	WriteFileOps(ctx context.Context, req FilesRequest) (FilesAnswer, error)
	SeeScreen(ctx context.Context, req SeeRequest) (SeeAnswer, error)
	ReadScreen(ctx context.Context, req ReadRequest) (ReadAnswer, error)
	Ping(ctx context.Context) error
}

const (
	KindProgram   = "program"
	KindRoute     = "route"
	KindAnswer    = "answer"
	KindAppAction = "app_action"
	KindWebAction = "web_action"
	KindUIAction  = "ui_action"
	KindStep      = "step"
	KindPlan      = "plan"
	KindStyle     = "style"
	KindWhichApp  = "which_app"
	KindText      = "text"
	KindFiles     = "files"
	KindSee       = "see"
	KindRead      = "read"
	KindPing      = "ping"
	KindFeedback  = "feedback"
	KindLearned   = "learned"
)

const (
	FeedbackFailedGuide  = "failed_guide"
	FeedbackNote         = "note"
	FeedbackStoppedGuide = "stopped_guide"
	MostFeedbackBytes    = 64 << 10
)

var StopReasons = []string{"Wrong step", "Too slow", "Just looking"}

const MostLearnedBytes = 64 << 10

type Learned struct {
	Habits []LearnedHabit `json:"habits"`
}

type LearnedHabit struct {
	What    string `json:"what"`
	When    string `json:"when,omitempty"`
	Days    int    `json:"days,omitempty"`
	Times   int    `json:"times,omitempty"`
	Yes     int    `json:"yes,omitempty"`
	Skips   int    `json:"skips,omitempty"`
	Always  bool   `json:"always,omitempty"`
	Stopped bool   `json:"stopped,omitempty"`
	Answer  string `json:"answer,omitempty"`
}

type Feedback struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type Asking struct {
	Kind    string          `json:"kind"`
	Request json.RawMessage `json:"request"`
}

type Answered struct {
	Answer json.RawMessage `json:"answer,omitempty"`
	Fault  *Fault          `json:"fault,omitempty"`
	Used   Used            `json:"used"`
}

const (
	FaultNoAnswer   = "no_answer"
	FaultUnwritable = "unwritable"
	FaultNoStep     = "no_step"
	FaultNotSetUp   = "not_set_up"
	FaultRefused    = "refused"
	FaultOverCap    = "over_cap"
	FaultFailed     = "failed"
)

type Fault struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	App     string `json:"app,omitempty"`
	Why     string `json:"why,omitempty"`
}

var ErrRefused = errors.New("synth: the Vspk server turned the request down")

var ErrOverCap = errors.New("synth: today's allowance is used up")

type faultError struct {
	fault Fault
	is    error
}

func (e *faultError) Error() string { return e.fault.Message }

func (e *faultError) Unwrap() error { return e.is }

func FaultOf(err error) *Fault {
	var unwritable *UnwritableError
	if errors.As(err, &unwritable) {
		return &Fault{Kind: FaultUnwritable, Message: err.Error(), App: unwritable.App, Why: unwritable.Why}
	}
	var empty *NoStepError
	if errors.As(err, &empty) {
		return &Fault{Kind: FaultNoStep, Message: err.Error(), Why: empty.Why}
	}
	var already *faultError
	if errors.As(err, &already) {
		f := already.fault
		return &f
	}
	switch {
	case errors.Is(err, ErrNoAnswer):
		return &Fault{Kind: FaultNoAnswer, Message: "the model did not answer in time"}
	case errors.Is(err, ErrNotSetUp):
		return &Fault{Kind: FaultNotSetUp, Message: "the Vspk server has no model set up"}
	}
	return &Fault{Kind: FaultFailed, Message: "the model could not answer this time"}
}

func (f *Fault) Err() error {
	switch f.Kind {
	case FaultUnwritable:
		return &UnwritableError{App: f.App, Why: f.Why}
	case FaultNoStep:
		return &NoStepError{Why: f.Why}
	case FaultNoAnswer:
		return &faultError{fault: *f, is: ErrNoAnswer}
	case FaultNotSetUp:
		return &faultError{fault: *f, is: ErrNotSetUp}
	case FaultRefused:
		return &faultError{fault: *f, is: ErrRefused}
	case FaultOverCap:
		return &faultError{fault: *f, is: ErrOverCap}
	}
	return &faultError{fault: *f}
}

var (
	_ Model = (*OpenAICompatible)(nil)
	_ Model = (*Live)(nil)
)
