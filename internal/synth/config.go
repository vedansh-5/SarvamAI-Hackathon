package synth

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Environment variables selecting the synthesis backend.
const (
	EnvProvider   = "VANS_LLM_PROVIDER" // "openai" (default) or "anthropic"
	EnvBaseURL    = "VANS_LLM_BASE_URL"
	EnvModel      = "VANS_LLM_MODEL"
	EnvAPIKey     = "VANS_LLM_API_KEY"
	EnvSchemaMode = "VANS_LLM_SCHEMA_MODE" // "json_schema" (default), "json_object", "prompt"

	// EnvFallbackModel names a second model to try when the first cannot
	// answer. Unset means there is nothing to fall back to.
	EnvFallbackModel = "VANS_LLM_FALLBACK_MODEL"

	EnvFastModel = "VANS_LLM_FAST_MODEL"

	// EnvTimeout is how long one request has to answer, as a Go duration.
	// Defaults to DefaultAnswerWait.
	EnvTimeout = "VANS_LLM_TIMEOUT"
)

// The wait is configurable because the default has to be short enough that a
// silent provider looks like a failure rather than a hang, and a slow
// open-weight model that spends the whole token budget thinking still has to be
// usable. Those two want different numbers, and only the person running it knows
// which model they pointed at.
func parseAnswerWait(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("synth: unreadable %s=%q, want a duration like \"45s\"", EnvTimeout, s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("synth: %s must be more than zero, got %q", EnvTimeout, s)
	}
	return d, nil
}

// ParseSchemaMode reads the response-constraint level from its env spelling.
//
// This is configurable because "OpenAI-compatible" is a claim about the
// endpoint shape, not about structured-output support. Providers differ on
// which response_format values they accept, and a provider that rejects
// json_schema returns an error that looks nothing like "unsupported" — so
// without a way to step down, an otherwise usable model is unreachable.
func ParseSchemaMode(s string) (SchemaMode, error) {
	switch s {
	case "", "json_schema", "schema":
		return SchemaJSON, nil
	case "json_object", "object":
		return SchemaObject, nil
	case "prompt", "none":
		return SchemaPrompt, nil
	default:
		return 0, fmt.Errorf("synth: unknown %s=%q, want \"json_schema\", \"json_object\" or \"prompt\"",
			EnvSchemaMode, s)
	}
}

// Defaults for the OpenAI-compatible backend.
//
// OpenRouter's free tier is the development default: no cost, open-weight
// models, and structured-output support.
//
// The 26b-a4b variant is chosen over the denser 31b for availability, not
// quality. Measured 2026-08-05: 31b was rate-limited upstream on every real
// synthesis request while trivially small requests still succeeded, so the
// limit is on load rather than on the account. 26b-a4b is a mixture with 4B
// active — weaker, but reachable. Override with VANS_LLM_MODEL when 31b has
// capacity.
const (
	DefaultBaseURL = "https://openrouter.ai/api/v1"
	DefaultOSModel = "google/gemma-4-26b-a4b-it:free"
)

// FromEnv builds a Synthesizer from the environment.
//
// Provider selection is config, not a code change — that is what the
// Synthesizer interface is for. The default is a free hosted open-weight
// model; set VANS_LLM_PROVIDER=anthropic to run against Claude instead.
func FromEnv() (Synthesizer, error) {
	switch os.Getenv(EnvProvider) {
	case "anthropic":
		return NewAnthropic(), nil

	case "", "openai", "openai-compatible":
		key := os.Getenv(EnvAPIKey)
		if key == "" {
			return nil, fmt.Errorf("synth: %s is not set", EnvAPIKey)
		}

		base := os.Getenv(EnvBaseURL)
		if base == "" {
			base = DefaultBaseURL
		}
		model := os.Getenv(EnvModel)
		if model == "" {
			model = DefaultOSModel
		}

		mode, err := ParseSchemaMode(os.Getenv(EnvSchemaMode))
		if err != nil {
			return nil, err
		}

		wait, err := parseAnswerWait(os.Getenv(EnvTimeout))
		if err != nil {
			return nil, err
		}

		return FromSettings(Settings{
			APIKey:   key,
			BaseURL:  base,
			Model:    model,
			Fallback: os.Getenv(EnvFallbackModel),
			Fast:     os.Getenv(EnvFastModel),
		}, wait, mode), nil

	default:
		return nil, fmt.Errorf("synth: unknown %s=%q, want \"openai\" or \"anthropic\"",
			EnvProvider, os.Getenv(EnvProvider))
	}
}

func WaitAndModeFromEnv() (time.Duration, SchemaMode) {
	mode, err := ParseSchemaMode(os.Getenv(EnvSchemaMode))
	if err != nil {
		mode, _ = ParseSchemaMode("")
	}
	wait, err := parseAnswerWait(os.Getenv(EnvTimeout))
	if err != nil {
		wait = 0
	}
	return wait, mode
}
