//go:build tester

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/vedansh/vans/internal/keychain"
	"github.com/vedansh/vans/internal/setup"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

var serverURL = ""

func loadEnv() {}

func chooseModel(context.Context, *store.Store) brain {
	remote := synth.NewRemote(serverURL, "")
	invite := &setup.Invite{Remote: remote, Keys: keychain.System{}}
	invite.Load()
	fmt.Fprintf(os.Stderr, "[model] asking the VANS server at %s\n", serverURL)
	return brain{synthesizer: remote, meter: remote.Usage, learner: remote, content: remote, trials: remote, invite: invite, ready: invite.Ready, reaches: serverURL}
}

func modelLabel() string { return "the VANS server" }
