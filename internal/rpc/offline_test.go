package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/engine"
	"github.com/vedansh/vans/internal/store"
)

func habitServer(t *testing.T, action store.Click) (*Server, *bytes.Buffer, *bool) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &engine.Engine{Clicks: st}
	timed := engine.Timed{Trigger: engine.Trigger{Kind: engine.WhenOpened}, Action: action, Days: 3}
	if err := e.KeepTimed(ctx, timed); err != nil {
		t.Fatal(err)
	}
	_ = st.SetSetting(ctx, "offers.greeted_on", time.Now().Format("2006-01-02"))
	up := false
	var out bytes.Buffer
	s := &Server{Engine: e, Online: func(context.Context) bool { return up }}
	s.out = json.NewEncoder(&out)
	return s, &out, &up
}

func TestAnOfferThatNeedsTheInternetWaitsForIt(t *testing.T) {
	ctx := context.Background()
	s, out, up := habitServer(t, store.Click{App: "Clockify", Role: "AXButton", Title: "Start"})

	s.offer(ctx, engine.Cause{Kind: engine.MacOpened})
	if strings.Contains(out.String(), `"offer"`) {
		t.Fatalf("offered while offline: %s", out.String())
	}

	s.offer(ctx, engine.Cause{Kind: engine.ByTheClock})
	if strings.Contains(out.String(), `"offer"`) {
		t.Fatalf("offered while still offline: %s", out.String())
	}

	*up = true
	s.offer(ctx, engine.Cause{Kind: engine.ByTheClock})
	if strings.Count(out.String(), `"method":"offer"`) != 1 || !strings.Contains(out.String(), "Clockify") {
		t.Fatalf("not offered once the internet was back: %s", out.String())
	}
}

func TestOpeningAnAppIsOfferedOffline(t *testing.T) {
	s, out, _ := habitServer(t, store.Click{App: "Spotify", Role: store.RoleFront})
	s.offer(context.Background(), engine.Cause{Kind: engine.MacOpened})
	if !strings.Contains(out.String(), `"method":"offer"`) {
		t.Fatalf("an app to open was held for the internet: %s", out.String())
	}
}

func TestAHeldOfferIsDroppedAfterTwoHours(t *testing.T) {
	s, out, up := habitServer(t, store.Click{App: "Clockify", Role: "AXButton", Title: "Start"})
	s.offer(context.Background(), engine.Cause{Kind: engine.MacOpened})
	s.mu.Lock()
	for i := range s.held {
		s.held[i].since = time.Now().Add(-holdOfflineFor - time.Minute)
	}
	s.mu.Unlock()
	*up = true
	s.offer(context.Background(), engine.Cause{Kind: engine.ByTheClock})
	if strings.Contains(out.String(), `"method":"offer"`) {
		t.Fatalf("a stale offer came back: %s", out.String())
	}
}
