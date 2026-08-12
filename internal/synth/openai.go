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

	// MaxTokens defaults to 4096. Programs are small; reasoning models
	// preamble before them.
	MaxTokens int

	// Retries is how many times a transient failure is retried. Defaults to
	// DefaultRetries. Free tiers rate-limit constantly and upstream capacity
	// comes and goes, so a 429 is a wait-and-retry, not a failed task.
	Retries int

	// RetryWait is the base backoff, doubled per attempt. Defaults to
	// DefaultRetryWait. A Retry-After header takes precedence.
	RetryWait time.Duration

	// HTTP defaults to a client with a 3-minute timeout.
	HTTP *http.Client
}

// Retry defaults for the transient-failure path.
const (
	DefaultRetries   = 3
	DefaultRetryWait = 2 * time.Second
)

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
	return &http.Client{Timeout: 3 * time.Minute}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
		maxTokens = 4096
	}

	system := systemPrompt
	if c.SchemaMode != SchemaJSON {
		// Without API-level schema enforcement the shape has to be carried by
		// the prompt instead.
		schema, err := json.MarshalIndent(extract.ProgramSchema(), "", "  ")
		if err != nil {
			return extract.Program{}, fmt.Errorf("synth: encode schema: %w", err)
		}
		system += "\n\nReply with a single JSON object and nothing else — no prose, no markdown fences. It must satisfy this JSON Schema:\n\n" + string(schema)
	}

	body := chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildPrompt(req)},
		},
		ResponseFormat: c.responseFormat(),
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

func (c *OpenAICompatible) responseFormat() json.RawMessage {
	switch c.SchemaMode {
	case SchemaJSON:
		wrapper := map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "extraction_program",
				"strict": true,
				"schema": extract.ProgramSchema(),
			},
		}
		b, err := json.Marshal(wrapper)
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

// post sends the request, retrying transient failures with backoff.
func (c *OpenAICompatible) post(ctx context.Context, body chatRequest) (string, error) {
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

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
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
		return "", fmt.Errorf("synth: %s returned %d: %s", url, resp.StatusCode, truncate(string(data), 300))
	}

	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("synth: decode response: %w (body %q)", err, truncate(string(data), 300))
	}
	if out.Error != nil {
		return "", fmt.Errorf("synth: provider error: %s", out.Error.Message)
	}
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
