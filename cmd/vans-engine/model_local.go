//go:build !tester

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vedansh/vans/internal/envfile"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/keychain"
	"github.com/vedansh/vans/internal/setup"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

func loadEnv() { envfile.Load(".env", "../.env") }

func chooseModel(ctx context.Context, st *store.Store) brain {
	if url := strings.TrimSpace(os.Getenv(synth.EnvServerURL)); url != "" {
		return fromServer(url)
	}
	if os.Getenv(synth.EnvProvider) == "anthropic" {
		s, err := synth.FromEnv()
		if err != nil {
			s = unconfigured{err: err}
		}
		return brain{synthesizer: s, ready: func() error { return err }}
	}
	llm := synth.NewLive()
	wait, mode := synth.WaitAndModeFromEnv()
	model := &setup.Model{
		Live: llm, Keys: keychain.System{}, Saved: st, Env: os.Getenv,
		Wait: wait, Mode: mode,
	}
	model.Load(ctx)
	return brain{
		synthesizer: llm, model: model, meter: llm.Meter, reaches: modelAddress(),
		ready: func() error {
			if !llm.Configured() {
				return synth.ErrNotSetUp
			}
			return nil
		},
	}
}

func modelAddress() string {
	if base := strings.TrimSpace(os.Getenv(synth.EnvBaseURL)); base != "" {
		return base
	}
	return synth.DefaultBaseURL
}

func fromServer(url string) brain {
	remote := synth.NewRemote(url, os.Getenv(synth.EnvInvite))
	invite := &setup.Invite{Remote: remote, Keys: keychain.System{}}
	invite.Load()
	fmt.Fprintf(os.Stderr, "[model] asking the VANS server at %s\n", url)
	return brain{synthesizer: remote, meter: remote.Usage, learner: remote, invite: invite, ready: invite.Ready, reaches: url}
}

type unconfigured struct{ err error }

func (u unconfigured) Synthesize(context.Context, synth.Request) (extract.Program, error) {
	return extract.Program{}, u.err
}

func modelLabel() string {
	if m := os.Getenv(synth.EnvModel); m != "" {
		return m
	}
	if os.Getenv(synth.EnvProvider) == "anthropic" {
		return synth.DefaultModel
	}
	return synth.DefaultOSModel
}
