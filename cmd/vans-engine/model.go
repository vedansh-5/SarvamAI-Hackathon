package main

import (
	"github.com/vedansh/vans/internal/engine"
	"github.com/vedansh/vans/internal/setup"
	"github.com/vedansh/vans/internal/synth"
)

type brain struct {
	synthesizer synth.Synthesizer
	model       *setup.Model
	meter       *synth.Usage
	learner     engine.LearnedSender
	invite      *setup.Invite
	ready       func() error
}
