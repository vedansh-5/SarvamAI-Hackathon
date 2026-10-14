package setup

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/keychain"
	"github.com/vedansh/vans/internal/synth"
)

type Saved interface {
	Setting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

const (
	keyService = "dev.vedansh.vans"
	keyAccount = "model-key"

	savedBaseURL  = "model.base_url"
	savedModel    = "model.name"
	savedFast     = "model.fast"
	savedFallback = "model.fallback"
	savedPriceIn  = "model.price_in"
	savedPriceOut = "model.price_out"
)

const (
	FromEnvironment = "environment"
	FromSaved       = "saved"
)

var TestWait = 25 * time.Second

const (
	GeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	GeminiModel   = "gemini-flash-lite-latest"
)

func whereTheKeyGoes(key, base, model string) (string, string) {
	base, model = strings.TrimSpace(base), strings.TrimSpace(model)
	if strings.HasPrefix(key, "sk-or-") {
		return base, model
	}
	if base == "" {
		base = GeminiBaseURL
	}
	if model == "" && base == GeminiBaseURL {
		model = GeminiModel
	}
	return base, model
}

type Model struct {
	Live  *synth.Live
	Keys  keychain.Store
	Saved Saved
	Env   func(string) string

	Wait time.Duration
	Mode synth.SchemaMode

	Test func(ctx context.Context, c *synth.OpenAICompatible) error

	source string
}

type Status struct {
	Configured bool    `json:"configured"`
	Source     string  `json:"source"`
	BaseURL    string  `json:"base_url"`
	Model      string  `json:"model"`
	Fast       string  `json:"fast_model"`
	Fallback   string  `json:"fallback_model"`
	HasKey     bool    `json:"has_key"`
	KeyHint    string  `json:"key_hint"`
	PriceIn    float64 `json:"price_in"`
	PriceOut   float64 `json:"price_out"`
}

type Change struct {
	APIKey   string  `json:"api_key"`
	BaseURL  string  `json:"base_url"`
	Model    string  `json:"model"`
	Fast     string  `json:"fast_model"`
	Fallback string  `json:"fallback_model"`
	PriceIn  float64 `json:"price_in"`
	PriceOut float64 `json:"price_out"`
}

func (m *Model) env(name string) string {
	if m.Env == nil {
		return ""
	}
	return strings.TrimSpace(m.Env(name))
}

func (m *Model) saved(ctx context.Context, key string) string {
	if m.Saved == nil {
		return ""
	}
	value, err := m.Saved.Setting(ctx, key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func (m *Model) storedKey() string {
	if m.Keys == nil {
		return ""
	}
	key, err := m.Keys.Get(keyService, keyAccount)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(key)
}

func (m *Model) fromSaved(ctx context.Context) synth.Settings {
	return synth.Settings{
		APIKey:   m.storedKey(),
		BaseURL:  m.saved(ctx, savedBaseURL),
		Model:    m.saved(ctx, savedModel),
		Fast:     m.saved(ctx, savedFast),
		Fallback: m.saved(ctx, savedFallback),
	}
}

func (m *Model) Load(ctx context.Context) {
	if key := m.env(synth.EnvAPIKey); key != "" {
		m.Live.Set(synth.FromSettings(synth.WithBackupFrom(m.env, synth.Settings{
			APIKey:   key,
			BaseURL:  m.env(synth.EnvBaseURL),
			Model:    m.env(synth.EnvModel),
			Fast:     m.env(synth.EnvFastModel),
			Fallback: m.env(synth.EnvFallbackModel),
		}), m.Wait, m.Mode))
		m.source = FromEnvironment
		return
	}
	if s := m.fromSaved(ctx); s.APIKey != "" {
		m.Live.Set(synth.FromSettings(synth.WithBackupFrom(m.env, s), m.Wait, m.Mode))
		m.source = FromSaved
	}
}

func hint(key string) string {
	if len(key) < 8 {
		return ""
	}
	return "…" + key[len(key)-4:]
}

func (m *Model) price(ctx context.Context, key string) float64 {
	v, err := strconv.ParseFloat(m.saved(ctx, key), 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func (m *Model) Status(ctx context.Context) Status {
	out := Status{
		Source:   m.source,
		PriceIn:  m.price(ctx, savedPriceIn),
		PriceOut: m.price(ctx, savedPriceOut),
	}
	if c := m.Live.Current(); c != nil {
		out.Configured = true
		out.BaseURL, out.Model, out.Fast, out.Fallback = c.BaseURL, c.Model, c.Fast, c.Fallback
		out.HasKey, out.KeyHint = c.APIKey != "", hint(c.APIKey)
		return out
	}
	s := m.fromSaved(ctx)
	out.BaseURL, out.Model, out.Fast, out.Fallback = s.BaseURL, s.Model, s.Fast, s.Fallback
	return out
}

func (m *Model) Save(ctx context.Context, change Change) (Status, error) {
	key := strings.TrimSpace(change.APIKey)
	if key == "" {
		if c := m.Live.Current(); c != nil {
			key = c.APIKey
		}
	}
	if key == "" {
		key = m.storedKey()
	}
	if key == "" {
		return m.Status(ctx), errors.New("setup: a key is needed before a model can be used")
	}
	if change.PriceIn < 0 || change.PriceOut < 0 {
		return m.Status(ctx), errors.New("setup: a price cannot be less than nothing")
	}

	change.BaseURL, change.Model = whereTheKeyGoes(key, change.BaseURL, change.Model)
	client := synth.FromSettings(synth.WithBackupFrom(m.env, synth.Settings{
		APIKey: key, BaseURL: change.BaseURL, Model: change.Model, Fast: change.Fast, Fallback: change.Fallback,
	}), m.Wait, m.Mode)

	test := m.Test
	if test == nil {
		test = func(ctx context.Context, c *synth.OpenAICompatible) error { return c.Ping(ctx) }
	}
	testing, cancel := context.WithTimeout(ctx, TestWait)
	err := test(testing, client)
	cancel()
	if err != nil {
		return m.Status(ctx), fmt.Errorf("setup: the model did not answer with that key and name, so nothing was saved: %w", err)
	}

	if strings.TrimSpace(change.APIKey) != "" {
		if m.Keys == nil {
			return m.Status(ctx), errors.New("setup: this build has nowhere safe to keep a key")
		}
		if err := m.Keys.Set(keyService, keyAccount, key); err != nil {
			return m.Status(ctx), fmt.Errorf("setup: the key could not be kept in the Keychain: %w", err)
		}
	}
	if m.Saved != nil {
		for name, value := range map[string]string{
			savedBaseURL:  strings.TrimSpace(change.BaseURL),
			savedModel:    strings.TrimSpace(change.Model),
			savedFast:     strings.TrimSpace(change.Fast),
			savedFallback: strings.TrimSpace(change.Fallback),
			savedPriceIn:  strconv.FormatFloat(change.PriceIn, 'f', -1, 64),
			savedPriceOut: strconv.FormatFloat(change.PriceOut, 'f', -1, 64),
		} {
			if err := m.Saved.SetSetting(ctx, name, value); err != nil {
				return m.Status(ctx), fmt.Errorf("setup: could not save %s: %w", name, err)
			}
		}
	}

	m.Live.Set(client)
	m.source = FromSaved
	return m.Status(ctx), nil
}

func (m *Model) Forget(ctx context.Context) (Status, error) {
	if m.Keys != nil {
		if err := m.Keys.Delete(keyService, keyAccount); err != nil {
			return m.Status(ctx), err
		}
	}
	if m.source != FromEnvironment {
		m.Live.Set(nil)
		m.source = ""
	}
	return m.Status(ctx), nil
}

func (m *Model) Cost(ctx context.Context, used synth.Used) float64 {
	in, out := m.price(ctx, savedPriceIn), m.price(ctx, savedPriceOut)
	if in == 0 && out == 0 {
		return 0
	}
	return (float64(used.In)*in + float64(used.Out)*out) / 1e6
}
