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

func TestSigningInWithAnEmailKeepsTheCodeItGives(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var check synth.JoinCheck
		_ = json.NewDecoder(r.Body).Decode(&check)
		switch {
		case r.URL.Path == synth.JoinAskPath:
			_ = json.NewEncoder(w).Encode(synth.Answered{})
		case r.URL.Path == synth.JoinCheckPath && check.Code == "123456":
			answer, _ := json.Marshal(synth.Joined{Invite: "vans-from-email", Waiting: true})
			_ = json.NewEncoder(w).Encode(synth.Answered{Answer: answer})
		case r.URL.Path == synth.JoinCheckPath:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(synth.Answered{Fault: &synth.Fault{Kind: synth.FaultRefused, Message: "That code isn't right."}})
		case r.Header.Get("Authorization") == "Bearer vans-right":
			_ = json.NewEncoder(w).Encode(synth.Answered{})
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(synth.Answered{Fault: &synth.Fault{Kind: synth.FaultRefused, Message: "no"}})
		}
	}))
	t.Cleanup(srv.Close)
	k := keys{}
	in := &Invite{Remote: synth.NewRemote(srv.URL, ""), Keys: k}

	if err := in.AskToJoin(ctx, " "); err == nil {
		t.Error("an empty email was sent")
	}
	if err := in.AskToJoin(ctx, "riya@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Join(ctx, "riya@example.com", "000000"); err == nil || in.Status().Has || len(k) != 0 {
		t.Fatalf("a wrong code: err %v, keychain %v", err, k)
	}
	status, err := in.Join(ctx, " Riya@Example.com ", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Has || !status.Waiting || status.Email != "riya@example.com" || in.Remote.InviteCode() != "vans-from-email" {
		t.Fatalf("status = %+v, code %q", status, in.Remote.InviteCode())
	}
	if k[keyService+"/"+inviteAccount] != "vans-from-email" || k[keyService+"/"+emailAccount] != "riya@example.com" {
		t.Fatalf("keychain %v", k)
	}

	again := &Invite{Remote: synth.NewRemote(srv.URL, ""), Keys: k}
	again.Load()
	if again.Status().Email != "riya@example.com" || !again.Status().Has {
		t.Errorf("after a restart: %+v", again.Status())
	}
	if _, err := again.Save(ctx, "vans-right"); err != nil {
		t.Fatal(err)
	}
	if again.Status().Email != "" || k[keyService+"/"+emailAccount] != "" {
		t.Errorf("a pasted code kept the old email: %+v, %v", again.Status(), k)
	}
}
