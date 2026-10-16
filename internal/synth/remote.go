package synth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vedansh/vans/internal/answer"
	"github.com/vedansh/vans/internal/appact"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/uiact"
	"github.com/vedansh/vans/internal/webact"
)

const (
	EnvServerURL = "VANS_SERVER_URL"
	EnvInvite    = "VANS_INVITE"
)

const (
	AskPath         = "/v1/ask"
	RemoteWait      = 150 * time.Second
	MostAnswerBytes = 8 << 20
)

type Remote struct {
	URL   string
	Code  string
	HTTP  *http.Client
	Usage *Usage

	mu sync.RWMutex
}

func NewRemote(url, code string) *Remote {
	return &Remote{URL: strings.TrimSpace(url), Code: strings.TrimSpace(code), Usage: &Usage{}}
}

var _ Model = (*Remote)(nil)

func (r *Remote) SetCode(code string) {
	r.mu.Lock()
	r.Code = strings.TrimSpace(code)
	r.mu.Unlock()
}

func (r *Remote) HasCode() bool { return r.InviteCode() != "" }

func (r *Remote) InviteCode() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Code
}

func (r *Remote) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{Timeout: RemoteWait}
}

func ask[Q, A any](ctx context.Context, r *Remote, kind string, req Q) (A, error) {
	var none A
	inner, err := json.Marshal(req)
	if err != nil {
		return none, fmt.Errorf("synth: encode %s request: %w", kind, err)
	}
	payload, err := json.Marshal(Asking{Kind: kind, Request: inner})
	if err != nil {
		return none, fmt.Errorf("synth: encode %s request: %w", kind, err)
	}

	url := strings.TrimSuffix(r.URL, "/") + AskPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return none, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+r.InviteCode())

	start := time.Now()
	resp, err := r.client().Do(httpReq)
	if err != nil {
		if ctx.Err() == nil && timedOut(err) {
			return none, &noAnswer{model: "the VANS server", waited: readableWait(time.Since(start))}
		}
		return none, fmt.Errorf("synth: the VANS server could not be reached: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, MostAnswerBytes))
	if err != nil {
		return none, fmt.Errorf("synth: read the VANS server's answer: %w", err)
	}
	var out Answered
	if err := json.Unmarshal(data, &out); err != nil {
		return none, fmt.Errorf("synth: the VANS server answered %d with something unreadable", resp.StatusCode)
	}
	r.Usage.addUsed(out.Used)
	if out.Fault != nil {
		return none, out.Fault.Err()
	}
	if resp.StatusCode != http.StatusOK {
		return none, fmt.Errorf("synth: the VANS server answered %d", resp.StatusCode)
	}

	var got A
	if len(out.Answer) > 0 {
		if err := json.Unmarshal(out.Answer, &got); err != nil {
			return none, fmt.Errorf("synth: decode %s answer: %w", kind, err)
		}
	}
	return got, nil
}

func (r *Remote) Synthesize(ctx context.Context, req Request) (extract.Program, error) {
	return ask[Request, extract.Program](ctx, r, KindProgram, req)
}

func (r *Remote) Decide(ctx context.Context, req route.Request) (route.Decision, error) {
	return ask[route.Request, route.Decision](ctx, r, KindRoute, req)
}

func (r *Remote) Answer(ctx context.Context, req AnswerRequest) (answer.Answer, error) {
	return ask[AnswerRequest, answer.Answer](ctx, r, KindAnswer, req)
}

func (r *Remote) WriteAction(ctx context.Context, req AppActRequest) (appact.Program, error) {
	return ask[AppActRequest, appact.Program](ctx, r, KindAppAction, req)
}

func (r *Remote) WriteWebAction(ctx context.Context, req WebActRequest) (webact.Program, error) {
	return ask[WebActRequest, webact.Program](ctx, r, KindWebAction, req)
}

func (r *Remote) WriteUIAction(ctx context.Context, req UIActRequest) (uiact.Program, error) {
	return ask[UIActRequest, uiact.Program](ctx, r, KindUIAction, req)
}

func (r *Remote) WriteStep(ctx context.Context, req StepRequest) (StepAnswer, error) {
	return ask[StepRequest, StepAnswer](ctx, r, KindStep, req)
}

func (r *Remote) WritePlan(ctx context.Context, req PlanRequest) (PlanAnswer, error) {
	return ask[PlanRequest, PlanAnswer](ctx, r, KindPlan, req)
}

func (r *Remote) WriteStyle(ctx context.Context, req StyleRequest) (StyleAnswer, error) {
	return ask[StyleRequest, StyleAnswer](ctx, r, KindStyle, req)
}

func (r *Remote) WhichApp(ctx context.Context, req AppChoiceRequest) (AppChoice, error) {
	return ask[AppChoiceRequest, AppChoice](ctx, r, KindWhichApp, req)
}

func (r *Remote) WorkOnText(ctx context.Context, req TextRequest) (TextAnswer, error) {
	return ask[TextRequest, TextAnswer](ctx, r, KindText, req)
}

func (r *Remote) WriteFileOps(ctx context.Context, req FilesRequest) (FilesAnswer, error) {
	return ask[FilesRequest, FilesAnswer](ctx, r, KindFiles, req)
}

func (r *Remote) SeeScreen(ctx context.Context, req SeeRequest) (SeeAnswer, error) {
	return ask[SeeRequest, SeeAnswer](ctx, r, KindSee, req)
}

func (r *Remote) ReadScreen(ctx context.Context, req ReadRequest) (ReadAnswer, error) {
	return ask[ReadRequest, ReadAnswer](ctx, r, KindRead, req)
}

func (r *Remote) SendFeedback(ctx context.Context, f Feedback) error {
	_, err := ask[Feedback, struct{}](ctx, r, KindFeedback, f)
	return err
}

func (r *Remote) SendLearned(ctx context.Context, l Learned) error {
	_, err := ask[Learned, struct{}](ctx, r, KindLearned, l)
	return err
}

func (r *Remote) Ping(ctx context.Context) error {
	_, err := ask[struct{}, struct{}](ctx, r, KindPing, struct{}{})
	return err
}
