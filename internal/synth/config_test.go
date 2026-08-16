package synth

import "testing"

func TestParseSchemaMode(t *testing.T) {
	cases := map[string]SchemaMode{
		"":            SchemaJSON,
		"json_schema": SchemaJSON,
		"schema":      SchemaJSON,
		"json_object": SchemaObject,
		"object":      SchemaObject,
		"prompt":      SchemaPrompt,
		"none":        SchemaPrompt,
	}
	for in, want := range cases {
		got, err := ParseSchemaMode(in)
		if err != nil {
			t.Errorf("ParseSchemaMode(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseSchemaMode(%q) = %v, want %v", in, got, want)
		}
	}

	// A typo must not silently fall back to the strictest mode and then fail
	// against a provider that never supported it.
	if _, err := ParseSchemaMode("json-schema"); err == nil {
		t.Error("an unknown mode was accepted")
	}
}

func TestFromEnvUsesTheSchemaMode(t *testing.T) {
	t.Setenv(EnvAPIKey, "test-key")
	t.Setenv(EnvSchemaMode, "json_object")

	s, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	c, ok := s.(*OpenAICompatible)
	if !ok {
		t.Fatalf("got %T, want *OpenAICompatible", s)
	}
	if c.SchemaMode != SchemaObject {
		t.Errorf("SchemaMode = %v, want SchemaObject", c.SchemaMode)
	}
}

func TestFromEnvRejectsABadSchemaMode(t *testing.T) {
	t.Setenv(EnvAPIKey, "test-key")
	t.Setenv(EnvSchemaMode, "strict")

	if _, err := FromEnv(); err == nil {
		t.Fatal("a bad schema mode was accepted")
	}
}

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv(EnvAPIKey, "test-key")

	s, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	c := s.(*OpenAICompatible)
	if c.BaseURL != DefaultBaseURL || c.Model != DefaultOSModel {
		t.Errorf("defaults not applied: %s %s", c.BaseURL, c.Model)
	}
	if c.SchemaMode != SchemaJSON {
		t.Errorf("SchemaMode = %v, want the strictest by default", c.SchemaMode)
	}
}

func TestFromEnvNeedsAKey(t *testing.T) {
	t.Setenv(EnvAPIKey, "")
	if _, err := FromEnv(); err == nil {
		t.Error("a backend was built with no API key")
	}
}
