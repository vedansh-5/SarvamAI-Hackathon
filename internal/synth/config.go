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
// models, and structured-output support. Gemma is dense and instruction-tuned,
// which makes it more reliable at emitting a bare JSON object than the
// reasoning-tuned mixtures, several of which prepend a thinking trace.
const (
	DefaultBaseURL = "https://openrouter.ai/api/v1"
	DefaultOSModel = "google/gemma-4-31b-it:free"
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
