package synth

import (
	"fmt"
	"os"
)

// Environment variables selecting the synthesis backend.
const (
	EnvProvider = "VANS_LLM_PROVIDER" // "openai" (default) or "anthropic"
	EnvBaseURL  = "VANS_LLM_BASE_URL"
	EnvModel    = "VANS_LLM_MODEL"
	EnvAPIKey   = "VANS_LLM_API_KEY"
)

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

		return &OpenAICompatible{BaseURL: base, APIKey: key, Model: model}, nil

	default:
		return nil, fmt.Errorf("synth: unknown %s=%q, want \"openai\" or \"anthropic\"",
			EnvProvider, os.Getenv(EnvProvider))
	}
}
