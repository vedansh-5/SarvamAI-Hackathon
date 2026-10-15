package setup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vedansh/vans/internal/synth"
)

func inviteServer(t *testing.T, good string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+good {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(synth.Answered{Fault: &synth.Fault{Kind: synth.FaultRefused, Message: "this invite code is not one VANS knows"}})
			return
		}
		_ = json.NewEncoder(w).Encode(synth.Answered{})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestARightInviteCodeIsKeptAndAWrongOneIsNot(t *testing.T) {
	ctx := context.Background()
	srv := inviteServer(t, "vans-right")
	k := keys{}
	in := &Invite{Remote: synth.NewRemote(srv.URL, ""), Keys: k}

	if err := in.Ready(); !errors.Is(err, ErrNoInvite) {
		t.Fatalf("Ready with no code = %v, want ErrNoInvite", err)
	}
	if _, err := in.Save(ctx, "vans-wrong"); !errors.Is(err, synth.ErrRefused) {
		t.Fatalf("a wrong code saved: %v", err)
	}
	if in.Status().Has || len(k) != 0 {
		t.Fatalf("a refused code was kept: status %+v, keychain %v", in.Status(), k)
	}

	if _, err := in.Save(ctx, "  vans-right \n"); err != nil {
		t.Fatal(err)
	}
	if !in.Status().Has || k[keyService+"/"+inviteAccount] != "vans-right" {
		t.Fatalf("the right code was not kept: keychain %v", k)
	}

	again := &Invite{Remote: synth.NewRemote(srv.URL, ""), Keys: k}
	again.Load()
	if err := again.Ready(); err != nil {
		t.Errorf("the code was not read back on the next start: %v", err)
	}
}
