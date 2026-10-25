package synth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	JoinAskPath   = "/v1/join/ask"
	JoinCheckPath = "/v1/join/check"
)

type JoinAsk struct {
	Email string `json:"email"`
}

type JoinCheck struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type Joined struct {
	Invite  string `json:"invite"`
	Waiting bool   `json:"waiting,omitempty"`
}

func (r *Remote) AskToJoin(ctx context.Context, email string) error {
	_, err := join[JoinAsk, struct{}](ctx, r, JoinAskPath, JoinAsk{Email: email})
	return err
}

func (r *Remote) CheckJoin(ctx context.Context, email, code string) (Joined, error) {
	return join[JoinCheck, Joined](ctx, r, JoinCheckPath, JoinCheck{Email: email, Code: code})
}

func join[Q, A any](ctx context.Context, r *Remote, path string, req Q) (A, error) {
	var none A
	payload, err := json.Marshal(req)
	if err != nil {
		return none, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(r.URL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return none, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := r.client().Do(httpReq)
	if err != nil {
		return none, fmt.Errorf("synth: the Vspk server could not be reached: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return none, fmt.Errorf("synth: read the Vspk server's answer: %w", err)
	}
	var out Answered
	if err := json.Unmarshal(data, &out); err != nil {
		return none, fmt.Errorf("synth: the Vspk server answered %d with something unreadable", resp.StatusCode)
	}
	if out.Fault != nil {
		return none, out.Fault.Err()
	}
	if resp.StatusCode != http.StatusOK {
		return none, fmt.Errorf("synth: the Vspk server answered %d", resp.StatusCode)
	}
	var got A
	if len(out.Answer) > 0 {
		if err := json.Unmarshal(out.Answer, &got); err != nil {
			return none, fmt.Errorf("synth: decode the join answer: %w", err)
		}
	}
	return got, nil
}
