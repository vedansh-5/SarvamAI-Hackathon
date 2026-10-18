package server

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
)

func TestTheConsoleDeletesATesterOnlyWhenTicked(t *testing.T) {
	srv, ledger, client := consoleServer(t)
	signIn(t, srv, client)
	_ = ledger.KeepLearned(context.Background(), "Riya", time.Now(), `[{"what":"Open Spotify"}]`)

	if status, _ := page(t, client, http.MethodPost, srv.URL+"/console/testers/delete", url.Values{"code": {riya}}); status != http.StatusBadRequest {
		t.Errorf("without the tick answered %d", status)
	}
	if _, ok, _ := ledger.Tester(context.Background(), riya); !ok {
		t.Fatal("deleted without the tick")
	}

	page(t, client, http.MethodPost, srv.URL+"/console/testers/delete", url.Values{"code": {riya}, "sure": {"yes"}})
	if _, ok, _ := ledger.Tester(context.Background(), riya); ok {
		t.Error("still there after deleting")
	}
	if ledger.Learned("Riya") != "" {
		t.Error("their habits were kept")
	}
	if _, err := synth.NewRemote(srv.URL, riya).Content(context.Background()); err == nil {
		t.Error("a deleted code still reaches the server")
	}
}

func TestTheTableDeletesATesterAndTheirHabits(t *testing.T) {
	d := &Dynamo{Client: newTable(), Table: "vans-test"}
	ctx := context.Background()
	if err := d.AddTester(ctx, Tester{Name: "Riya", Code: riya, Daily: 5}); err != nil {
		t.Fatal(err)
	}
	_ = d.KeepLearned(ctx, "Riya", time.Now(), `[{"what":"Open Spotify"}]`)
	if err := d.DeleteTester(ctx, riya); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := d.Tester(ctx, riya); ok {
		t.Error("the tester is still there")
	}
	if h, _ := d.Habits(ctx, "Riya"); len(h.List) != 0 {
		t.Errorf("habits kept: %+v", h)
	}
	if err := d.DeleteTester(ctx, riya); err != ErrNoTester {
		t.Errorf("deleting twice gave %v", err)
	}
}
