package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
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

func TestTheListChangesManyTestersAtOnce(t *testing.T) {
	runVoiceTrial(t)
	srv, ledger, client := consoleServer(t)
	signIn(t, srv, client)
	ctx := context.Background()
	const asha, dev = "asha-code-0123456789", "dev-code-01234567890"
	_ = ledger.AddTester(ctx, Tester{Name: "Asha", Code: asha, Daily: 10})
	_ = ledger.AddTester(ctx, Tester{Name: "Dev", Code: dev, Daily: 10})
	many := func(v url.Values) (int, string) {
		return page(t, client, http.MethodPost, srv.URL+"/console/testers/many", v)
	}
	who := func(code string) Tester {
		got, _, _ := ledger.Tester(ctx, code)
		return got
	}

	if _, body := many(url.Values{"do": {"off"}}); !strings.Contains(body, "Tick at least one tester") {
		t.Errorf("with nobody ticked:\n%s", body)
	}

	many(url.Values{"do": {"off"}, "code": {riya, asha}})
	if !who(riya).Off || !who(asha).Off || who(dev).Off {
		t.Error("switch off did not reach exactly the ticked testers")
	}
	many(url.Values{"do": {"daily"}, "daily": {"50"}, "code": {asha, dev}})
	if who(asha).Daily != 50 || who(dev).Daily != 50 || who(riya).Daily != 10 {
		t.Error("the allowance did not reach exactly the ticked testers")
	}
	many(url.Values{"do": {"trial"}, "trial": {"voice=on"}, "code": {riya, dev}})
	if who(riya).Trials["voice"] != "on" || who(dev).Trials["voice"] != "on" || who(asha).Trials["voice"] != "" {
		t.Error("the Setup version did not reach exactly the ticked testers")
	}
	if _, body := many(url.Values{"do": {"trial"}, "trial": {"voice=loud"}, "code": {asha}}); !strings.Contains(body, "not a version") {
		t.Errorf("an unknown version:\n%s", body)
	}

	many(url.Values{"do": {"delete"}, "code": {riya, dev}})
	if _, ok, _ := ledger.Tester(ctx, riya); !ok {
		t.Fatal("removed without the tick")
	}
	many(url.Values{"do": {"delete"}, "sure": {"yes"}, "code": {riya, dev}})
	_, riyaThere, _ := ledger.Tester(ctx, riya)
	_, devThere, _ := ledger.Tester(ctx, dev)
	_, ashaThere, _ := ledger.Tester(ctx, asha)
	if riyaThere || devThere || !ashaThere {
		t.Errorf("after removing Riya and Dev: Riya %v, Dev %v, Asha %v", riyaThere, devThere, ashaThere)
	}
	if _, body := page(t, client, http.MethodGet, srv.URL+"/console/testers", nil); !strings.Contains(body, `name="code" value="`+asha+`"`) || !strings.Contains(body, "voice=off") {
		t.Errorf("the list has no tick box or versions for Asha:\n%s", body)
	}
}
