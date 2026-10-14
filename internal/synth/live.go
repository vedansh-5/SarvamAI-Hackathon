package synth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vedansh/vans/internal/answer"
	"github.com/vedansh/vans/internal/appact"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/uiact"
	"github.com/vedansh/vans/internal/webact"
)

type Usage struct {
	calls atomic.Int64
	in    atomic.Int64
	out   atomic.Int64
}

type Used struct {
	Calls int64 `json:"calls"`
	In    int64 `json:"in"`
	Out   int64 `json:"out"`
}

func (u *Usage) add(in, out int64) {
	if u == nil {
		return
	}
	u.calls.Add(1)
	u.in.Add(in)
	u.out.Add(out)
}

func (u *Usage) addUsed(d Used) {
	if u == nil {
		return
	}
	u.calls.Add(d.Calls)
	u.in.Add(d.In)
	u.out.Add(d.Out)
}

func (u *Usage) Now() Used {
	if u == nil {
		return Used{}
	}
	return Used{Calls: u.calls.Load(), In: u.in.Load(), Out: u.out.Load()}
}

func (a Used) Since(b Used) Used {
	return Used{Calls: a.Calls - b.Calls, In: a.In - b.In, Out: a.Out - b.Out}
}

type Settings struct {
	APIKey   string
	BaseURL  string
	Model    string
	Fast     string
	Fallback string

	BackupBaseURL string
	BackupKey     string
	BackupModel   string
	BackupMode    SchemaMode
}

func FromSettings(s Settings, wait time.Duration, mode SchemaMode) *OpenAICompatible {
	base := strings.TrimSpace(s.BaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	model := strings.TrimSpace(s.Model)
	if model == "" {
		model = DefaultOSModel
	}
	c := &OpenAICompatible{
		BaseURL:    base,
		APIKey:     strings.TrimSpace(s.APIKey),
		Model:      model,
		Fallback:   strings.TrimSpace(s.Fallback),
		Fast:       strings.TrimSpace(s.Fast),
		AnswerWait: wait,
		SchemaMode: mode,
	}
	backupBase, backupKey, backupModel := strings.TrimSpace(s.BackupBaseURL), strings.TrimSpace(s.BackupKey), strings.TrimSpace(s.BackupModel)
	if backupBase != "" && backupKey != "" && backupModel != "" {
		c.Backup = &OpenAICompatible{
			BaseURL:    strings.TrimSuffix(backupBase, "/"),
			APIKey:     backupKey,
			Model:      backupModel,
			AnswerWait: wait,
			SchemaMode: s.BackupMode,
		}
	}
	return c
}

func WithBackupFrom(env func(string) string, s Settings) Settings {
	s.BackupBaseURL = strings.TrimSpace(env(EnvBackupBaseURL))
	s.BackupKey = strings.TrimSpace(env(EnvBackupAPIKey))
	s.BackupModel = strings.TrimSpace(env(EnvBackupModel))
	s.BackupMode = SchemaObject
	if spelled := strings.TrimSpace(env(EnvBackupSchemaMode)); spelled != "" {
		if mode, err := ParseSchemaMode(spelled); err == nil {
			s.BackupMode = mode
		}
	}
	return s
}

var ErrNotSetUp = errors.New("synth: no model is set up yet; add a key in Setup")

func (c *OpenAICompatible) Ping(ctx context.Context) error {
	if c.BaseURL == "" || c.Model == "" {
		return fmt.Errorf("synth: BaseURL and Model are required")
	}
	_, err := c.post(ctx, chatRequest{
		Model:     c.Model,
		MaxTokens: 400,
		Messages:  []chatMessage{{Role: "user", Content: "Reply with the single word: ready"}},
	})
	return err
}

type Live struct {
	now   atomic.Pointer[OpenAICompatible]
	Meter *Usage
}

func NewLive() *Live { return &Live{Meter: &Usage{}} }

func (l *Live) Set(c *OpenAICompatible) {
	if c != nil {
		c.Usage = l.Meter
	}
	l.now.Store(c)
}

func (l *Live) Configured() bool { return l.now.Load() != nil }

func (l *Live) Current() *OpenAICompatible { return l.now.Load() }

func (l *Live) use() (*OpenAICompatible, error) {
	if c := l.now.Load(); c != nil {
		return c, nil
	}
	return nil, ErrNotSetUp
}

func (l *Live) Synthesize(ctx context.Context, req Request) (extract.Program, error) {
	c, err := l.use()
	if err != nil {
		return extract.Program{}, err
	}
	return c.Synthesize(ctx, req)
}

func (l *Live) Decide(ctx context.Context, req route.Request) (route.Decision, error) {
	c, err := l.use()
	if err != nil {
		return route.Decision{}, err
	}
	return c.Decide(ctx, req)
}

func (l *Live) Answer(ctx context.Context, req AnswerRequest) (answer.Answer, error) {
	c, err := l.use()
	if err != nil {
		return answer.Answer{}, err
	}
	return c.Answer(ctx, req)
}

func (l *Live) WriteAction(ctx context.Context, req AppActRequest) (appact.Program, error) {
	c, err := l.use()
	if err != nil {
		return appact.Program{}, err
	}
	return c.WriteAction(ctx, req)
}

func (l *Live) WriteWebAction(ctx context.Context, req WebActRequest) (webact.Program, error) {
	c, err := l.use()
	if err != nil {
		return webact.Program{}, err
	}
	return c.WriteWebAction(ctx, req)
}

func (l *Live) WriteUIAction(ctx context.Context, req UIActRequest) (uiact.Program, error) {
	c, err := l.use()
	if err != nil {
		return uiact.Program{}, err
	}
	return c.WriteUIAction(ctx, req)
}

func (l *Live) WriteStep(ctx context.Context, req StepRequest) (StepAnswer, error) {
	c, err := l.use()
	if err != nil {
		return StepAnswer{}, err
	}
	return c.WriteStep(ctx, req)
}

func (l *Live) WritePlan(ctx context.Context, req PlanRequest) (PlanAnswer, error) {
	c, err := l.use()
	if err != nil {
		return PlanAnswer{}, err
	}
	return c.WritePlan(ctx, req)
}

func (l *Live) WriteStyle(ctx context.Context, req StyleRequest) (StyleAnswer, error) {
	c, err := l.use()
	if err != nil {
		return StyleAnswer{}, err
	}
	return c.WriteStyle(ctx, req)
}

func (l *Live) WhichApp(ctx context.Context, req AppChoiceRequest) (AppChoice, error) {
	c, err := l.use()
	if err != nil {
		return AppChoice{}, err
	}
	return c.WhichApp(ctx, req)
}

func (l *Live) WorkOnText(ctx context.Context, req TextRequest) (TextAnswer, error) {
	c, err := l.use()
	if err != nil {
		return TextAnswer{}, err
	}
	return c.WorkOnText(ctx, req)
}

func (l *Live) WriteFileOps(ctx context.Context, req FilesRequest) (FilesAnswer, error) {
	c, err := l.use()
	if err != nil {
		return FilesAnswer{}, err
	}
	return c.WriteFileOps(ctx, req)
}

func (l *Live) SeeScreen(ctx context.Context, req SeeRequest) (SeeAnswer, error) {
	c, err := l.use()
	if err != nil {
		return SeeAnswer{}, err
	}
	return c.SeeScreen(ctx, req)
}

func (l *Live) ReadScreen(ctx context.Context, req ReadRequest) (ReadAnswer, error) {
	c, err := l.use()
	if err != nil {
		return ReadAnswer{}, err
	}
	return c.ReadScreen(ctx, req)
}

func (l *Live) Ping(ctx context.Context) error {
	c, err := l.use()
	if err != nil {
		return err
	}
	return c.Ping(ctx)
}
