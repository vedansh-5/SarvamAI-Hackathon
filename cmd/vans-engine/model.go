package main

import (
	"context"
	"net"
	"net/url"
	"time"

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
	reaches     string
}

func reachable(address string) func(context.Context) bool {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return nil
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "443")
	}
	return func(ctx context.Context) bool {
		var d net.Dialer
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		conn, err := d.DialContext(ctx, "tcp", host)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}
}
