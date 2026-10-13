package synth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/extract"
)

// OpenAICompatible synthesizes programs against any OpenAI-shaped chat
// completions endpoint — OpenRouter, Groq, Cerebras, Together, vLLM, or a
// local server. Only the base URL and model name change.
//
// This exists so synthesis can run on free and open-weight models. Those are
// meaningfully worse at this task than a frontier model, which the runtime
// absorbs rather than hides: a bad program fails verification and the failure
// ladder re-synthesizes it. The cost shows up as slower cold runs, not as
// wrong data.
type OpenAICompatible struct {
	// BaseURL is the API root, without a trailing slash.
	// OpenRouter: https://openrouter.ai/api/v1
	// Groq:       https://api.groq.com/openai/v1
	BaseURL string

	APIKey string
	Model  string

	// SchemaMode controls how the response shape is constrained.
	SchemaMode SchemaMode

	// MaxTokens defaults to DefaultMaxTokens — the same allowance the
	// Anthropic path uses, and for the same reason. The budget is not for the
	// answer, which is a few hundred bytes, but for the thinking several of
	// the free models emit before reaching it, billed to the same allowance.
	// At 4096 responses were cut off mid-reasoning and never got to the JSON,
	// surfacing as a token limit rather than as anything resembling the real
	// problem.
	MaxTokens int

	// Retries is how many times a transient failure is retried. Defaults to
	// DefaultRetries. Free tiers rate-limit constantly and upstream capacity
	// comes and goes, so a 429 is a wait-and-retry, not a failed task.
	Retries int

	// RetryWait is the base backoff, doubled per attempt. Defaults to
	// DefaultRetryWait. A Retry-After header takes precedence.
	RetryWait time.Duration

	// Fallback is a second model, tried when the first one cannot answer at
	// all: it went silent, it stayed overloaded past the retry budget, or the
	// provider does not have it under that name. Empty by default — the right
	// second model depends on the account and the endpoint, so a guessed one
	// would be worse than none.
	Fallback string

	Fast string

	// AnswerWait bounds a single request. Defaults to DefaultAnswerWait.
	AnswerWait time.Duration

	// HTTP, when set, is used as it is, and its own timeout governs instead of
	// AnswerWait.
	HTTP *http.Client

	Usage *Usage
}

var Notice = func(format string, args ...any) {}

// Retry defaults for the transient-failure path.
const (
	DefaultRetries   = 3
	DefaultRetryWait = 2 * time.Second
)

// DefaultAnswerWait bounds one request to the provider.
//
// Measured 2026-09-22: an overloaded model accepted the connection and then
// never answered. The allowance here was three minutes, and the step writer
// re-asks once, so a silent model held the panel on "working out step 1" for
// six — which from the outside is indistinguishable from the app having hung. A
// healthy answer to one of these requests comes back in a second or two. Raise
// it with VANS_LLM_TIMEOUT for a model slow enough to need the room.
const DefaultAnswerWait = 45 * time.Second

// ErrNoAnswer is the provider taking the request and never replying, which is
// not the same as refusing it. Re-asking costs the whole wait over again and has
// nothing new to offer, so the callers that would otherwise re-ask test for this
// instead.
var ErrNoAnswer = errors.New("the model did not answer")

type noAnswer struct {
	model  string
	waited time.Duration
}

func (e *noAnswer) Error() string {
	return fmt.Sprintf("synth: %s did not answer in %s", e.model, e.waited)
}

func (e *noAnswer) Unwrap() error { return ErrNoAnswer }

// A wait of 45 seconds reads better as "45s", and one of two milliseconds must
// not read as "0s" — that is the message denying that anything was waited for.
func readableWait(d time.Duration) time.Duration {
	if d >= 10*time.Second {
		return d.Round(time.Second)
	}
	return d.Round(time.Millisecond)
}

// providerRefusal is a reply the provider turned down outright. The status is
// kept because 404 means the model does not exist under this name, which a
// different model can answer, while the rest mean the request itself is wrong.
type providerRefusal struct {
	url    string
	status int
	body   string
}

func (e *providerRefusal) Error() string {
	return fmt.Sprintf("synth: %s returned %d: %s", e.url, e.status, truncate(e.body, 300))
}

// transientError marks a failure worth retrying: rate limits and upstream
// capacity problems. A 4xx that is not 429 means the request itself is wrong,
// and retrying it only wastes quota.
type transientError struct {
	status     int
	retryAfter time.Duration
	body       string
}

func (e *transientError) Error() string {
	return fmt.Sprintf("status %d: %s", e.status, truncate(e.body, 300))
}

// SchemaMode selects how strongly the response is constrained.
type SchemaMode int

const (
	// SchemaJSON sends response_format.json_schema. Strongest guarantee;
	// supported by a subset of models.
	SchemaJSON SchemaMode = iota

	// SchemaObject sends response_format.json_object and puts the schema in
	// the prompt. Use when a model rejects json_schema.
	SchemaObject

	// SchemaPrompt constrains nothing at the API level. Last resort.
	SchemaPrompt
)

func (c *OpenAICompatible) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: c.answerWait()}
}

func (c *OpenAICompatible) answerWait() time.Duration {
	if c.AnswerWait > 0 {
		return c.AnswerWait
	}
	return DefaultAnswerWait
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	Image string `json:"-"`
}

func (m chatMessage) MarshalJSON() ([]byte, error) {
	if m.Image == "" {
		return json.Marshal(struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{m.Role, m.Content})
	}
	type part map[string]any
	return json.Marshal(struct {
		Role    string `json:"role"`
		Content []part `json:"content"`
	}{m.Role, []part{
		{"type": "text", "text": m.Content},
		{"type": "image_url", "image_url": map[string]string{"url": m.Image}},
	}})
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat json.RawMessage `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *OpenAICompatible) Synthesize(ctx context.Context, req Request) (extract.Program, error) {
	if len(req.Candidates) == 0 {
		return extract.Program{}, fmt.Errorf("synth: no candidates to choose from")
	}
	if c.BaseURL == "" || c.Model == "" {
		return extract.Program{}, fmt.Errorf("synth: BaseURL and Model are required")
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = int(DefaultMaxTokens)
	}

	system, err := c.systemFor(systemPrompt, "extraction", extract.ProgramSchema)
	if err != nil {
		return extract.Program{}, err
	}

	body := chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildPrompt(req)},
		},
		ResponseFormat: c.responseFormat("extraction_program", extract.ProgramSchema, true),
	}

	raw, err := c.post(ctx, body)
	if err != nil {
		return extract.Program{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return extract.Program{}, fmt.Errorf("synth: no JSON object in response (got %q)", truncate(raw, 300))
	}

	var p extract.Program
	if err := json.Unmarshal([]byte(text), &p); err != nil {
		return extract.Program{}, fmt.Errorf("synth: decode program: %w (output %q)", err, truncate(text, 300))
	}
	if err := p.Validate(); err != nil {
		return extract.Program{}, fmt.Errorf("synth: %w", err)
	}
	return p, nil
}

// post sends the request, retrying transient failures with backoff.
const schemaPreamble = "\n\nReply with a single JSON object and nothing else — no prose, no markdown fences. It must satisfy this JSON Schema:\n\n"

// Providers differ on which response_format values they accept, so a build
// that cannot enforce a schema at the API level carries it in the prompt.
func (c *OpenAICompatible) systemFor(base, label string, schema func() map[string]any) (string, error) {
	if c.SchemaMode == SchemaJSON {
		return base, nil
	}
	encoded, err := json.MarshalIndent(schema(), "", "  ")
	if err != nil {
		return "", fmt.Errorf("synth: encode %s schema: %w", label, err)
	}
	return base + schemaPreamble + string(encoded), nil
}

func (c *OpenAICompatible) responseFormat(name string, schema func() map[string]any, strict bool) json.RawMessage {
	switch c.SchemaMode {
	case SchemaJSON:
		b, err := json.Marshal(map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   name,
				"strict": strict,
				"schema": schema(),
			},
		})
		if err != nil {
			return nil
		}
		return b

	case SchemaObject:
		return json.RawMessage(`{"type":"json_object"}`)

	default:
		return nil
	}
}

// post sends the request, moving to the fallback model when the first one
// cannot answer at all. Transient failures are retried within each model.
func (c *OpenAICompatible) post(ctx context.Context, body chatRequest) (string, error) {
	var failures []error

	for _, model := range c.ladder(body.Model) {
		body.Model = model

		text, err := c.postRetrying(ctx, body)
		if err == nil {
			return text, nil
		}
		failures = append(failures, err)

		if !anotherModelMightAnswer(err) {
			break
		}
	}
	return "", errors.Join(failures...)
}

func (c *OpenAICompatible) ladder(first string) []string {
	rungs := []string{first}
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		for _, have := range rungs {
			if have == model {
				return
			}
		}
		rungs = append(rungs, model)
	}
	if first != c.Model && first == strings.TrimSpace(c.Fast) {
		add(c.Model)
	}
	add(c.Fallback)
	return rungs
}

func (c *OpenAICompatible) easy() string {
	if fast := strings.TrimSpace(c.Fast); fast != "" {
		return fast
	}
	return c.Model
}

// Worth a second model when the first could not answer: it went silent, it is
// overloaded past our patience, or the provider does not have it under that
// name. A request the provider rejected as malformed is ours to fix, and every
// model will reject it the same way.
func anotherModelMightAnswer(err error) bool {
	if errors.Is(err, ErrNoAnswer) {
		return true
	}
	var transient *transientError
	if errors.As(err, &transient) {
		return true
	}
	var refused *providerRefusal
	return errors.As(err, &refused) && refused.status == http.StatusNotFound
}

func (c *OpenAICompatible) postRetrying(ctx context.Context, body chatRequest) (string, error) {
	retries := c.Retries
	if retries == 0 {
		retries = DefaultRetries
	}
	wait := c.RetryWait
	if wait == 0 {
		wait = DefaultRetryWait
	}

	var last error
	for attempt := 0; ; attempt++ {
		text, err := c.postOnce(ctx, body)
		if err == nil {
			return text, nil
		}

		var tr *transientError
		if !errors.As(err, &tr) || attempt >= retries {
			if tr != nil {
				return "", fmt.Errorf("synth: %w (gave up after %d attempts)", err, attempt+1)
			}
			return "", err
		}
		last = err

		delay := tr.retryAfter
		if delay <= 0 {
			delay = wait << attempt
		}
		Notice("%s could not answer (status %d); asking again in %s, attempt %d of %d",
			body.Model, tr.status, readableWait(delay), attempt+2, retries+1)
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("synth: %w while backing off from: %v", ctx.Err(), last)
		case <-time.After(delay):
		}
	}
}

func (c *OpenAICompatible) postOnce(ctx context.Context, body chatRequest) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("synth: encode request: %w", err)
	}

	url := strings.TrimSuffix(c.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	start := time.Now()
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		// Our own timeout and the caller giving up arrive here looking alike,
		// and the difference is the whole point: one is the model failing to
		// answer, which the fallback exists for, and the other is someone
		// pressing Stop.
		if ctx.Err() == nil && timedOut(err) {
			return "", &noAnswer{model: body.Model, waited: readableWait(time.Since(start))}
		}
		return "", fmt.Errorf("synth: %s: %w", url, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("synth: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Free tiers rate-limit aggressively, and the body says which limit
		// was hit — surface it rather than a bare status code.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return "", &transientError{
				status:     resp.StatusCode,
				retryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
				body:       string(data),
			}
		}
		return "", &providerRefusal{url: url, status: resp.StatusCode, body: string(data)}
	}

	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("synth: decode response: %w (body %q)", err, truncate(string(data), 300))
	}
	if out.Error != nil {
		return "", fmt.Errorf("synth: provider error: %s", out.Error.Message)
	}
	c.Usage.add(out.Usage.PromptTokens, out.Usage.CompletionTokens)
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("synth: response had no choices")
	}
	if r := out.Choices[0].FinishReason; r == "length" {
		return "", fmt.Errorf("synth: response hit the token limit before completing")
	}

	return out.Choices[0].Message.Content, nil
}

// extractJSONObject pulls the first balanced JSON object out of a reply.
//
// Open-weight models routinely wrap their answer in markdown fences or lead
// with a sentence of commentary even when told not to, and reasoning-tuned
// models prepend a whole thinking trace. Failing on that would send perfectly
// good programs into the failure ladder for a formatting reason.
func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	if json.Valid([]byte(s)) && strings.HasPrefix(s, "{") {
		return s
	}

	// Scan for a balanced object, ignoring braces inside strings.
	depth := 0
	start := -1
	inString := false
	escaped := false

	for i, r := range s {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case inString:
			// nothing
		case r == '{':
			if depth == 0 {
				start = i
			}
			depth++
		case r == '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					candidate := s[start : i+1]
					if json.Valid([]byte(candidate)) {
						return candidate
					}
					start = -1
				}
			}
		}
	}
	return ""
}

func timedOut(err error) bool {
	var deadline interface{ Timeout() bool }
	return errors.As(err, &deadline) && deadline.Timeout()
}

// parseRetryAfter reads the header in its delay-seconds form. The HTTP-date
// form is ignored rather than guessed at — falling back to our own backoff is
// safer than trusting a clock we do not control.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs < 0 {
		return 0
	}
	const maxWait = 60 * time.Second
	d := time.Duration(secs) * time.Second
	if d > maxWait {
		return maxWait
	}
	return d
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
