package rpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/keychain"
	"github.com/vedansh/vans/internal/setup"
	"github.com/vedansh/vans/internal/synth"
)

type memoryKeys map[string]string

func (k memoryKeys) Get(service, account string) (string, error) {
	v, ok := k[service+"/"+account]
	if !ok {
		return "", keychain.ErrNotFound
	}
	return v, nil
}
func (k memoryKeys) Set(service, account, secret string) error {
	k[service+"/"+account] = secret
	return nil
}
func (k memoryKeys) Delete(service, account string) error {
	delete(k, service+"/"+account)
	return nil
}

type memorySettings map[string]string

func (m memorySettings) Setting(_ context.Context, key string) (string, error) { return m[key], nil }
func (m memorySettings) SetSetting(_ context.Context, key, value string) error {
	m[key] = value
	return nil
}

func withModel(answers error) func(*Server, *session) {
	return func(srv *Server, _ *session) {
		srv.Model = &setup.Model{
			Live: synth.NewLive(), Keys: memoryKeys{}, Saved: memorySettings{},
			Test: func(context.Context, *synth.OpenAICompatible) error { return answers },
		}
	}
}

func TestAKeyIsSavedThroughThePanelAndNeverEchoedBack(t *testing.T) {
	if before := decode[setup.Status](t, run(t, withModel(nil), req(1, "model_settings", nil)).ok(1)); before.Configured {
		t.Errorf("status = %+v before anything was saved", before)
	}

	s := run(t, withModel(nil),
		req(2, "set_model_settings", setup.Change{APIKey: "sk-secret-1234", Model: "a-model"}),
	)
	raw := s.ok(2)
	after := decode[setup.Status](t, raw)
	if !after.Configured || after.Model != "a-model" || !after.HasKey {
		t.Errorf("status = %+v, want the model set up", after)
	}
	if strings.Contains(string(raw), "sk-secret") {
		t.Errorf("the key came back to the window: %s", raw)
	}
}

func TestAKeyTheModelRefusesIsReportedAsNotSaved(t *testing.T) {
	s := run(t, withModel(errors.New("401")),
		req(1, "set_model_settings", setup.Change{APIKey: "sk-wrong-0000"}),
	)

	e := s.failure(1)
	if e.Code != CodeModelNotSaved || !strings.Contains(e.Message, "nothing was saved") {
		t.Errorf("error = %+v, want it to say nothing was saved", e)
	}
}
