package setup

import (
	"context"
	"errors"
	"strings"

	"github.com/vedansh/vans/internal/keychain"
	"github.com/vedansh/vans/internal/synth"
)

const (
	inviteAccount = "invite-code"
	emailAccount  = "invite-email"
)

var ErrNoInvite = errors.New("setup: this copy of VANS needs its invite code; add it in Setup")

type Invite struct {
	Remote *synth.Remote
	Keys   keychain.Store
	email  string
}

type InviteStatus struct {
	Has     bool   `json:"has"`
	Email   string `json:"email,omitempty"`
	Waiting bool   `json:"waiting,omitempty"`
}

func (i *Invite) Load() {
	if i.Remote.HasCode() {
		return
	}
	if code, err := i.Keys.Get(keyService, inviteAccount); err == nil {
		i.Remote.SetCode(code)
		i.email, _ = i.Keys.Get(keyService, emailAccount)
	}
}

func (i *Invite) Status() InviteStatus {
	return InviteStatus{Has: i.Remote.HasCode(), Email: i.email}
}

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
	i.email = ""
	_ = i.Keys.Delete(keyService, emailAccount)
	return i.Status(), nil
}

func (i *Invite) AskToJoin(ctx context.Context, email string) error {
	if strings.TrimSpace(email) == "" {
		return errors.New("Type your email first.")
	}
	return i.Remote.AskToJoin(ctx, strings.TrimSpace(email))
}

func (i *Invite) Join(ctx context.Context, email, code string) (InviteStatus, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if strings.TrimSpace(code) == "" {
		return i.Status(), errors.New("Type the code from the email first.")
	}
	joined, err := i.Remote.CheckJoin(ctx, email, code)
	if err != nil {
		return i.Status(), err
	}
	if err := i.Keys.Set(keyService, inviteAccount, joined.Invite); err != nil {
		return i.Status(), err
	}
	_ = i.Keys.Set(keyService, emailAccount, email)
	i.Remote.SetCode(joined.Invite)
	i.email = email
	status := i.Status()
	status.Waiting = joined.Waiting
	return status, nil
}
