package synth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const goodProgramJSON = `{
  "root": "div.results > article.card",
  "fields": [
    {"name": "title", "selector": "h3 a", "attr": "text"},
    {"name": "url", "selector": "h3 a", "attr": "href"}
  ],
  "expect": {"min_rows": 1, "required": ["title"]}
}`

// stubServer replies with content and records the request it received.
func stubServer(t *testing.T, status int, content string) (*httptest.Server, *chatRequest) {
	t.Helper()
	var got chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(content))
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": content}, "finish_reason": "stop"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	return srv, &got
}

func clientFor(srv *httptest.Server) *OpenAICompatible {
	return &OpenAICompatible{BaseURL: srv.URL, APIKey: "test-key", Model: "test/model"}
}

func TestOpenAISynthesizes(t *testing.T) {
	srv, req := stubServer(t, http.StatusOK, goodProgramJSON)

	got, err := clientFor(srv).Synthesize(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if got.Root != "div.results > article.card" {
		t.Errorf("root = %q", got.Root)
	}
	if len(got.Fields) != 2 {
		t.Errorf("got %d fields", len(got.Fields))
	}

	if req.Model != "test/model" {
		t.Errorf("model = %q", req.Model)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != "system" {
		t.Errorf("messages = %+v", req.Messages)
	}
	if !strings.Contains(req.Messages[1].Content, "the product names and prices") {
		t.Error("user message did not carry the intent")
	}
}

func TestOpenAISendsJSONSchemaByDefault(t *testing.T) {
	srv, req := stubServer(t, http.StatusOK, goodProgramJSON)

	if _, err := clientFor(srv).Synthesize(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	var rf map[string]any
	if err := json.Unmarshal(req.ResponseFormat, &rf); err != nil {
		t.Fatalf("response_format not sent: %v", err)
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v", rf["type"])
	}
	js, _ := rf["json_schema"].(map[string]any)
	if js["strict"] != true {
		t.Errorf("json_schema.strict = %v, want true", js["strict"])
	}
	if _, ok := js["schema"].(map[string]any); !ok {
		t.Error("json_schema.schema missing")
	}
}

// When the API can't enforce the shape, the prompt has to. Otherwise a model
// in SchemaObject mode is told to emit JSON with no idea which JSON.
func TestSchemaObjectModePutsSchemaInPrompt(t *testing.T) {
	srv, req := stubServer(t, http.StatusOK, goodProgramJSON)

	c := clientFor(srv)
	c.SchemaMode = SchemaObject
	if _, err := c.Synthesize(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if string(req.ResponseFormat) != `{"type":"json_object"}` {
		t.Errorf("response_format = %s", req.ResponseFormat)
	}
	if !strings.Contains(req.Messages[0].Content, "min_rows") {
		t.Error("system prompt did not carry the schema")
	}
}

func TestSchemaPromptModeSendsNoResponseFormat(t *testing.T) {
	srv, req := stubServer(t, http.StatusOK, goodProgramJSON)

	c := clientFor(srv)
	c.SchemaMode = SchemaPrompt
	if _, err := c.Synthesize(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if len(req.ResponseFormat) != 0 {
		t.Errorf("response_format = %s, want none", req.ResponseFormat)
	}
}

// Open-weight models wrap JSON in fences and prepend commentary even when told
// not to. Failing on that would push good programs into the failure ladder for
// a formatting reason.
func TestToleratesMessyReplies(t *testing.T) {
	cases := map[string]string{
		"markdown fence":   "```json\n" + goodProgramJSON + "\n```",
		"bare fence":       "```\n" + goodProgramJSON + "\n```",
		"leading prose":    "Sure! Here is the program:\n\n" + goodProgramJSON,
		"trailing prose":   goodProgramJSON + "\n\nLet me know if you need changes.",
		"thinking preface": "<think>The cards look right.</think>\n" + goodProgramJSON,
		"surrounded":       "Here you go:\n```json\n" + goodProgramJSON + "\n```\nHope that helps!",
	}

	for name, reply := range cases {
		srv, _ := stubServer(t, http.StatusOK, reply)
		got, err := clientFor(srv).Synthesize(context.Background(), sampleRequest())
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got.Root == "" {
			t.Errorf("%s: empty program", name)
		}
	}
}

// A brace inside a selector string must not end the object early.
func TestExtractJSONObjectIgnoresBracesInStrings(t *testing.T) {
	src := `prefix {"root": "div[data-x=\"}{\"] > a", "n": 1} suffix`
	got := extractJSONObject(src)

	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("extracted %q, which does not parse: %v", got, err)
	}
	if m["n"] != float64(1) {
		t.Errorf("extracted the wrong object: %q", got)
	}
}

func TestExtractJSONObjectEmptyCases(t *testing.T) {
	for _, s := range []string{"", "   ", "no json here", "{unbalanced", "}{"} {
		if got := extractJSONObject(s); got != "" {
			t.Errorf("extractJSONObject(%q) = %q, want empty", s, got)
		}
	}
}

// A schema-valid program can still be unusable. The provider guarantees shape;
// Validate guarantees it can run.
func TestRejectsValidJSONThatIsNotAValidProgram(t *testing.T) {
	srv, _ := stubServer(t, http.StatusOK, `{"root":"","fields":[],"expect":{"min_rows":0,"required":[]}}`)

	if _, err := clientFor(srv).Synthesize(context.Background(), sampleRequest()); err == nil {
		t.Error("accepted a program with no root and no fields")
	}
}

// Free tiers rate-limit constantly, and the body says which limit was hit.
func TestSurfacesHTTPErrorBody(t *testing.T) {
	srv, _ := stubServer(t, http.StatusTooManyRequests, `{"error":{"message":"free-models-per-day limit reached"}}`)

	_, err := clientFor(srv).Synthesize(context.Background(), sampleRequest())
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "free-models-per-day") {
		t.Errorf("error lost the provider's explanation: %v", err)
	}
}

func TestSurfacesProviderErrorInBody(t *testing.T) {
	srv, _ := stubServer(t, http.StatusOK, "")
	// Replace the handler to return a 200 carrying an error object.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"model is offline","type":"server_error"}}`))
	})

	_, err := clientFor(srv).Synthesize(context.Background(), sampleRequest())
	if err == nil || !strings.Contains(err.Error(), "model is offline") {
		t.Errorf("got %v", err)
	}
}

func TestTruncatedResponseIsAnError(t *testing.T) {
	srv, _ := stubServer(t, http.StatusOK, "")
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"root\":\"a"},"finish_reason":"length"}]}`))
	})

	_, err := clientFor(srv).Synthesize(context.Background(), sampleRequest())
	if err == nil || !strings.Contains(err.Error(), "token limit") {
		t.Errorf("got %v, want a token-limit error", err)
	}
}

func TestRequiresBaseURLAndModel(t *testing.T) {
	if _, err := (&OpenAICompatible{Model: "m"}).Synthesize(context.Background(), sampleRequest()); err == nil {
		t.Error("want an error with no BaseURL")
	}
	if _, err := (&OpenAICompatible{BaseURL: "http://x"}).Synthesize(context.Background(), sampleRequest()); err == nil {
		t.Error("want an error with no Model")
	}
}

func TestSendsBearerToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": goodProgramJSON}}},
		})
	}))
	defer srv.Close()

	if _, err := clientFor(srv).Synthesize(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if auth != "Bearer test-key" {
		t.Errorf("Authorization = %q", auth)
	}
}

func TestFromEnvSelectsProvider(t *testing.T) {
	t.Setenv(EnvProvider, "")
	t.Setenv(EnvAPIKey, "k")
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvModel, "")

	s, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	oc, ok := s.(*OpenAICompatible)
	if !ok {
		t.Fatalf("got %T, want *OpenAICompatible", s)
	}
	if oc.BaseURL != DefaultBaseURL || oc.Model != DefaultOSModel {
		t.Errorf("defaults not applied: %+v", oc)
	}

	t.Setenv(EnvProvider, "anthropic")
	if s, err := FromEnv(); err != nil {
		t.Fatalf("FromEnv(anthropic): %v", err)
	} else if _, ok := s.(*Anthropic); !ok {
		t.Errorf("got %T, want *Anthropic", s)
	}

	t.Setenv(EnvProvider, "nonsense")
	if _, err := FromEnv(); err == nil {
		t.Error("want an error for an unknown provider")
	}
}

func TestFromEnvRequiresKey(t *testing.T) {
	t.Setenv(EnvProvider, "openai")
	t.Setenv(EnvAPIKey, "")

	if _, err := FromEnv(); err == nil {
		t.Error("want an error when the API key is unset")
	}
}
