package setup

import (
	"context"
	"errors"
	"strings"

	"github.com/vedansh/vans/internal/keychain"
	"github.com/vedansh/vans/internal/synth"
)

const inviteAccount = "invite-code"

var ErrNoInvite = errors.New("setup: this copy of VANS needs its invite code; add it in Setup")

type Invite struct {
	Remote *synth.Remote
	Keys   keychain.Store
}

type InviteStatus struct {
	Has bool `json:"has"`
}

func (i *Invite) Load() {
	if i.Remote.HasCode() {
		return
	}
	if code, err := i.Keys.Get(keyService, inviteAccount); err == nil {
		i.Remote.SetCode(code)
	}
}

func (i *Invite) Status() InviteStatus { return InviteStatus{Has: i.Remote.HasCode()} }

func (i *Invite) Ready() error {
	if !i.Remote.HasCode() {
		return ErrNoInvite
	}
	return nil
}

func (i *Invite) Save(ctx context.Context, code string) (InviteStatus, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return i.Status(), errors.New("setup: the invite code is empty")
	}
	before := i.Remote.InviteCode()
	i.Remote.SetCode(code)
	if err := i.Remote.Ping(ctx); err != nil {
		i.Remote.SetCode(before)
		return i.Status(), err
	}
	if err := i.Keys.Set(keyService, inviteAccount, code); err != nil {
		return i.Status(), err
	}
	return i.Status(), nil
}
