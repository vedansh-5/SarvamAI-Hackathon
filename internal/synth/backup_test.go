package synth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func overloaded(t *testing.T, asked *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func withBackup(primary, backup *httptest.Server, mode SchemaMode) *OpenAICompatible {
	c := clientFor(primary)
	c.Fallback = "test/fallback"
	c.RetryWait = time.Millisecond
	c.Backup = &OpenAICompatible{BaseURL: backup.URL, APIKey: "other-key", Model: "other/model", SchemaMode: mode}
	return c
}

func TestAsksTheBackupWhenTheFirstProviderIsOverloaded(t *testing.T) {
	var asked atomic.Int32
	backup, got := stubServer(t, http.StatusOK, goodProgramJSON)
	c := withBackup(overloaded(t, &asked), backup, SchemaObject)

	if _, err := c.Synthesize(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if n := asked.Load(); n != 4 {
		t.Errorf("first provider asked %d times, want 4 (two models, one retry each)", n)
	}
	if got.Model != "other/model" {
		t.Errorf("backup was asked for %q", got.Model)
	}
	var format struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(got.ResponseFormat, &format); err != nil || format.Type != "json_object" {
		t.Errorf("backup response_format = %s, want json_object", got.ResponseFormat)
	}
	if len(got.Messages) == 0 || got.Messages[0].Role != "system" || !strings.Contains(got.Messages[0].Content, "JSON Schema") {
		t.Error("backup's system message does not carry the schema")
	}
}

func TestKeepsTheSchemaForABackupThatTakesIt(t *testing.T) {
	var asked atomic.Int32
	backup, got := stubServer(t, http.StatusOK, goodProgramJSON)
	c := withBackup(overloaded(t, &asked), backup, SchemaJSON)

	if _, err := c.Synthesize(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if !strings.Contains(string(got.ResponseFormat), "json_schema") {
		t.Errorf("backup response_format = %s, want json_schema", got.ResponseFormat)
	}
}

func TestDoesNotAskTheBackupAboutARequestThatWasRefused(t *testing.T) {
	refusing, _ := stubServer(t, http.StatusBadRequest, `{"error":{"message":"bad request"}}`)
	var backupAsked atomic.Int32
	backup := overloaded(t, &backupAsked)
	c := withBackup(refusing, backup, SchemaObject)

	if _, err := c.Synthesize(context.Background(), sampleRequest()); err == nil {
		t.Fatal("want an error")
	}
	if n := backupAsked.Load(); n != 0 {
		t.Errorf("backup asked %d times, want 0", n)
	}
}

func TestABackupNeedsAllThreeSettings(t *testing.T) {
	env := map[string]string{
		EnvBackupBaseURL: "https://api.example.com/v1/",
		EnvBackupAPIKey:  "k",
		EnvBackupModel:   "m",
	}
	get := func(name string) string { return env[name] }

	c := FromSettings(WithBackupFrom(get, Settings{APIKey: "a", Model: "first"}), 0, SchemaJSON)
	if c.Backup == nil {
		t.Fatal("want a backup")
	}
	if c.Backup.BaseURL != "https://api.example.com/v1" || c.Backup.SchemaMode != SchemaObject {
		t.Errorf("backup = %q mode %d", c.Backup.BaseURL, c.Backup.SchemaMode)
	}

	delete(env, EnvBackupModel)
	if c := FromSettings(WithBackupFrom(get, Settings{APIKey: "a"}), 0, SchemaJSON); c.Backup != nil {
		t.Error("a backup without a model was built")
	}
}
