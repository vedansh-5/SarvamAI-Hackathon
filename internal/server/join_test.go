package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
)

type inbox struct {
	to    []string
	codes []string
}

func (i *inbox) Send(_ context.Context, to, subject, body string) error {
	i.to = append(i.to, to)
	i.codes = append(i.codes, strings.TrimPrefix(subject, "Your VANS code: "))
	if !strings.Contains(body, i.codes[len(i.codes)-1]) {
		return errors.New("the code is not in the body")
	}
	return nil
}

func (i *inbox) last() string { return i.codes[len(i.codes)-1] }

type joinBench struct {
	remote *synth.Remote
	ledger *Memory
	box    *inbox
	mail   *postbox
	now    *time.Time
	srv    *Server
}

func newJoinBench(t *testing.T, joins Joins) *joinBench {
	t.Helper()
	ledger := NewMemory(nil)
	if joins == nil {
		joins = ledger
	}
	now := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	b := &joinBench{ledger: ledger, box: &inbox{}, mail: &postbox{}, now: &now}
	b.srv = &Server{
		Model:   func(*synth.Usage) synth.Model { return &standIn{} },
		Ledger:  ledger,
		Mail:    b.mail,
		Joining: &Joining{Joins: joins, Send: b.box, Daily: JoinDaily, PerDay: 2},
		Now:     func() time.Time { return *b.now },
	}
	http := httptest.NewServer(b.srv)
	t.Cleanup(http.Close)
	b.remote = synth.NewRemote(http.URL, "")
	return b
}

func TestAnEmailCodeMakesATesterWithTheDefaultAllowance(t *testing.T) {
	ctx := context.Background()
	b := newJoinBench(t, nil)
	if err := b.remote.AskToJoin(ctx, "  Riya@Example.com "); err != nil {
		t.Fatal(err)
	}
	if len(b.box.to) != 1 || b.box.to[0] != "riya@example.com" || len(b.box.last()) != 6 {
		t.Fatalf("sent %v %v", b.box.to, b.box.codes)
	}
	joined, err := b.remote.CheckJoin(ctx, "riya@example.com", b.box.last())
	if err != nil || !strings.HasPrefix(joined.Invite, "vans-") || joined.Waiting {
		t.Fatalf("joined = %+v, %v", joined, err)
	}
	who, ok, _ := b.ledger.Tester(ctx, joined.Invite)
	if !ok || who.Name != "riya@example.com" || who.Email != "riya@example.com" || who.Daily != 100 || who.Off {
		t.Fatalf("tester = %+v", who)
	}
	if len(b.mail.subjects) != 1 || !strings.Contains(b.mail.subjects[0], "riya@example.com joined") {
		t.Errorf("Vedansh was told %v", b.mail.subjects)
	}
	b.remote.SetCode(joined.Invite)
	if err := b.remote.Ping(ctx); err != nil {
		t.Errorf("the new code cannot ask: %v", err)
	}
	if _, err := b.remote.CheckJoin(ctx, "riya@example.com", b.box.last()); err == nil {
		t.Error("a code worked twice")
	}

	*b.now = b.now.Add(2 * time.Minute)
	if err := b.remote.AskToJoin(ctx, "riya@example.com"); err != nil {
		t.Fatal(err)
	}
	again, err := b.remote.CheckJoin(ctx, "riya@example.com", b.box.last())
	if err != nil || again.Invite != joined.Invite {
		t.Errorf("signing in again gave %+v, %v; want the same code", again, err)
	}
}

func TestWrongAndOldCodesAreTurnedDown(t *testing.T) {
	ctx := context.Background()
	b := newJoinBench(t, nil)
	if err := b.remote.AskToJoin(ctx, "riya@example.com"); err != nil {
		t.Fatal(err)
	}
	right := b.box.last()
	wrong := "000000"
	if right == wrong {
		wrong = "111111"
	}
	for i := 0; i < MostTries; i++ {
		if _, err := b.remote.CheckJoin(ctx, "riya@example.com", wrong); err == nil || !strings.Contains(err.Error(), "isn't right") {
			t.Fatalf("try %d: err = %v", i, err)
		}
	}
	if _, err := b.remote.CheckJoin(ctx, "riya@example.com", right); err == nil || !strings.Contains(err.Error(), "Too many wrong tries") {
		t.Errorf("after %d wrong tries the right code gave %v", MostTries, err)
	}

	*b.now = b.now.Add(2 * time.Minute)
	if err := b.remote.AskToJoin(ctx, "riya@example.com"); err != nil {
		t.Fatal(err)
	}
	*b.now = b.now.Add(CodeLasts + time.Second)
	if _, err := b.remote.CheckJoin(ctx, "riya@example.com", b.box.last()); err == nil || !strings.Contains(err.Error(), "run out") {
		t.Errorf("an old code gave %v", err)
	}
}

func TestAskingAgainSoonSendsNothingNew(t *testing.T) {
	ctx := context.Background()
	b := newJoinBench(t, nil)
	for i := 0; i < 3; i++ {
		if err := b.remote.AskToJoin(ctx, "riya@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if len(b.box.codes) != 1 {
		t.Fatalf("%d emails within a minute; want 1", len(b.box.codes))
	}
	for i := 1; i < MostSends; i++ {
		*b.now = b.now.Add(ResendAfter)
		if err := b.remote.AskToJoin(ctx, "riya@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	*b.now = b.now.Add(ResendAfter)
	if err := b.remote.AskToJoin(ctx, "riya@example.com"); err == nil || !strings.Contains(err.Error(), "enough codes") {
		t.Errorf("email %d: err = %v", MostSends+1, err)
	}
	*b.now = b.now.Add(24 * time.Hour)
	if err := b.remote.AskToJoin(ctx, "riya@example.com"); err != nil {
		t.Errorf("the next day: %v", err)
	}
}

func TestPastTheDaysPlacesPeopleWait(t *testing.T) {
	ctx := context.Background()
	b := newJoinBench(t, nil)
	var codes []synth.Joined
	for _, email := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		if err := b.remote.AskToJoin(ctx, email); err != nil {
			t.Fatal(err)
		}
		joined, err := b.remote.CheckJoin(ctx, email, b.box.last())
		if err != nil {
			t.Fatal(err)
		}
		codes = append(codes, joined)
	}
	if codes[0].Waiting || codes[1].Waiting || !codes[2].Waiting {
		t.Fatalf("waiting = %v %v %v; want only the third", codes[0].Waiting, codes[1].Waiting, codes[2].Waiting)
	}
	b.remote.SetCode(codes[2].Invite)
	if err := b.remote.Ping(ctx); err == nil || !strings.Contains(err.Error(), "waiting list") {
		t.Fatalf("a waiting code asked: %v", err)
	}
	if err := b.ledger.SetOff(ctx, codes[2].Invite, false); err != nil {
		t.Fatal(err)
	}
	if err := b.remote.Ping(ctx); err != nil {
		t.Errorf("once let in: %v", err)
	}
	if who, _, _ := b.ledger.Tester(ctx, codes[2].Invite); who.Waiting {
		t.Error("still marked waiting after being let in")
	}
}

func TestBadAndThrowawayEmailsAreTurnedDown(t *testing.T) {
	ctx := context.Background()
	b := newJoinBench(t, nil)
	b.srv.Joining.HasMail = func(_ context.Context, domain string) bool { return domain != "nomail.example" }
	for email, want := range map[string]string{
		"riya":                 "doesn't look like",
		"riya@localhost":       "doesn't look like",
		"Riya <r@example.com>": "doesn't look like",
		"r@mailinator.com":     "throwaway",
		"r@nomail.example":     "can't receive mail",
	} {
		if err := b.remote.AskToJoin(ctx, email); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v; want %q", email, err, want)
		}
	}
	if len(b.box.codes) != 0 {
		t.Errorf("sent %d emails", len(b.box.codes))
	}
}

func TestOneNetworkCanOnlySignUpSoMany(t *testing.T) {
	ctx := context.Background()
	b := newJoinBench(t, nil)
	for i := 0; i < MostFromOneNet; i++ {
		if err := b.remote.AskToJoin(ctx, string(rune('a'+i))+"@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.remote.AskToJoin(ctx, "late@example.com"); err == nil || !strings.Contains(err.Error(), "this network") {
		t.Errorf("err = %v", err)
	}
}

func TestSigningUpIsClosedWithoutASender(t *testing.T) {
	b := newJoinBench(t, nil)
	b.srv.Joining = nil
	if err := b.remote.AskToJoin(context.Background(), "riya@example.com"); err == nil || !strings.Contains(err.Error(), "isn't open") {
		t.Errorf("err = %v", err)
	}
}

func TestTheTableKeepsSignUps(t *testing.T) {
	ctx := context.Background()
	d := &Dynamo{Client: newTable(), Table: "vans"}
	b := newJoinBench(t, d)
	b.srv.Ledger = d
	if err := b.remote.AskToJoin(ctx, "riya@example.com"); err != nil {
		t.Fatal(err)
	}
	joined, err := b.remote.CheckJoin(ctx, "riya@example.com", b.box.last())
	if err != nil {
		t.Fatal(err)
	}
	who, ok, err := d.Joined(ctx, "riya@example.com")
	if err != nil || !ok || who.Code != joined.Invite || who.Email != "riya@example.com" || who.Daily != JoinDaily {
		t.Fatalf("Joined = %+v, %v, %v", who, ok, err)
	}
	if err := d.Join(ctx, Tester{Name: "x", Email: "riya@example.com", Code: NewCode(), Daily: 1}); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("a second join for one email: %v", err)
	}
	if err := d.DeleteTester(ctx, joined.Invite); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := d.Joined(ctx, "riya@example.com"); ok {
		t.Error("the email still points at a deleted tester")
	}
	waiting := Tester{Name: "w@example.com", Email: "w@example.com", Code: NewCode(), Daily: 1, Off: true, Waiting: true}
	if err := d.Join(ctx, waiting); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := d.Tester(ctx, waiting.Code); !got.Waiting || !got.Off {
		t.Fatalf("kept as %+v", got)
	}
	if err := d.SetOff(ctx, waiting.Code, false); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := d.Tester(ctx, waiting.Code); got.Waiting || got.Off {
		t.Errorf("let in, kept as %+v", got)
	}
	if err := d.SetOff(ctx, "vans-nobody-has-this", false); !errors.Is(err, ErrNoTester) {
		t.Errorf("letting in nobody: %v", err)
	}
}
