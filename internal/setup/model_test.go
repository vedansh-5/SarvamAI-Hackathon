package setup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/keychain"
	"github.com/vedansh/vans/internal/synth"
)

type keys map[string]string

func (k keys) Get(service, account string) (string, error) {
	v, ok := k[service+"/"+account]
	if !ok {
		return "", keychain.ErrNotFound
	}
	return v, nil
}
func (k keys) Set(service, account, secret string) error { k[service+"/"+account] = secret; return nil }
func (k keys) Delete(service, account string) error      { delete(k, service+"/"+account); return nil }

type saved map[string]string

func (s saved) Setting(_ context.Context, key string) (string, error) { return s[key], nil }
func (s saved) SetSetting(_ context.Context, key, value string) error { s[key] = value; return nil }

func model(env map[string]string) (*Model, keys, saved) {
	k, s := keys{}, saved{}
	return &Model{
		Live: synth.NewLive(), Keys: k, Saved: s,
		Env:  func(name string) string { return env[name] },
		Test: func(context.Context, *synth.OpenAICompatible) error { return nil },
	}, k, s
}

func TestWithNothingSetUpTheModelIsNotConfigured(t *testing.T) {
	m, _, _ := model(nil)
	m.Load(context.Background())

	if st := m.Status(context.Background()); st.Configured || st.HasKey {
		t.Errorf("status = %+v, want nothing configured", st)
	}
	if _, err := m.Live.WriteStep(context.Background(), synth.StepRequest{}); !errors.Is(err, synth.ErrNotSetUp) {
		t.Errorf("err = %v, want the model reported as not set up", err)
	}
}

func TestAKeyThatWorksIsKeptInTheKeychainAndUsedAtOnce(t *testing.T) {
	m, k, s := model(nil)
	m.Load(context.Background())

	st, err := m.Save(context.Background(), Change{APIKey: "  sk-secret-1234  ", Model: "a-model", PriceIn: 0.1, PriceOut: 0.4})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !st.Configured || st.Source != FromSaved || st.Model != "a-model" || st.KeyHint != "…1234" {
		t.Errorf("status = %+v, want it configured from what was saved, with only the end of the key shown", st)
	}
	if got := k[keyService+"/"+keyAccount]; got != "sk-secret-1234" {
		t.Errorf("keychain holds %q, want the key", got)
	}
	for name, value := range s {
		if strings.Contains(value, "sk-secret") {
			t.Errorf("the key was written to the settings table under %s", name)
		}
	}

	again, _, _ := model(nil)
	again.Keys, again.Saved = k, s
	again.Load(context.Background())
	if st := again.Status(context.Background()); !st.Configured || st.Model != "a-model" {
		t.Errorf("status after a restart = %+v, want it still configured", st)
	}
}

func TestAKeyThatDoesNotWorkIsNotSaved(t *testing.T) {
	m, k, _ := model(nil)
	m.Test = func(context.Context, *synth.OpenAICompatible) error { return errors.New("401 unauthorised") }

	st, err := m.Save(context.Background(), Change{APIKey: "sk-wrong-9999"})
	if err == nil || !strings.Contains(err.Error(), "nothing was saved") {
		t.Fatalf("err = %v, want the key refused", err)
	}
	if st.Configured || len(k) != 0 {
		t.Errorf("status = %+v, keychain = %v; a key that failed must leave nothing behind", st, k)
	}
}

func TestTheModelCanBeChangedWithoutGivingTheKeyAgain(t *testing.T) {
	m, k, _ := model(nil)
	if _, err := m.Save(context.Background(), Change{APIKey: "sk-secret-1234", Model: "first"}); err != nil {
		t.Fatal(err)
	}
	st, err := m.Save(context.Background(), Change{Model: "second"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if st.Model != "second" || k[keyService+"/"+keyAccount] != "sk-secret-1234" {
		t.Errorf("status = %+v, keychain = %v; want the new model with the same key", st, k)
	}
}

func TestTheEnvironmentWinsAtStartup(t *testing.T) {
	m, k, _ := model(map[string]string{synth.EnvAPIKey: "sk-from-env-0000", synth.EnvModel: "env-model"})
	k[keyService+"/"+keyAccount] = "sk-saved-1111"
	m.Load(context.Background())

	st := m.Status(context.Background())
	if st.Source != FromEnvironment || st.Model != "env-model" || st.KeyHint != "…0000" {
		t.Errorf("status = %+v, want the environment's settings", st)
	}
}

func TestCostIsOnlyGivenWhenPricesAreSet(t *testing.T) {
	m, _, s := model(nil)
	used := synth.Used{Calls: 3, In: 2_000_000, Out: 500_000}

	if got := m.Cost(context.Background(), used); got != 0 {
		t.Errorf("cost = %v with no prices set, want none claimed", got)
	}
	s[savedPriceIn], s[savedPriceOut] = "0.1", "0.4"
	if got := m.Cost(context.Background(), used); got < 0.399 || got > 0.401 {
		t.Errorf("cost = %v, want 0.40", got)
	}
}

func TestForgettingRemovesTheKeyAndTurnsTheModelOff(t *testing.T) {
	m, k, _ := model(nil)
	if _, err := m.Save(context.Background(), Change{APIKey: "sk-secret-1234"}); err != nil {
		t.Fatal(err)
	}
	st, err := m.Forget(context.Background())
	if err != nil || st.Configured || len(k) != 0 {
		t.Errorf("status = %+v, keychain = %v, err = %v; want the key gone and the model off", st, k, err)
	}
}

func TestAKeyWithNothingElseSaidGoesToTheUsualModel(t *testing.T) {
	m, _, _ := model(nil)
	st, err := m.Save(context.Background(), Change{APIKey: "AIzaSyExample-key-0000"})
	if err != nil {
		t.Fatal(err)
	}
	if st.BaseURL != GeminiBaseURL || st.Model != GeminiModel {
		t.Errorf("status = %+v, want the usual endpoint and model filled in", st)
	}

	other, _, _ := model(nil)
	st, err = other.Save(context.Background(), Change{APIKey: "sk-or-v1-example-0000"})
	if err != nil {
		t.Fatal(err)
	}
	if st.BaseURL == GeminiBaseURL {
		t.Errorf("status = %+v, an OpenRouter key was pointed at another provider", st)
	}
}
