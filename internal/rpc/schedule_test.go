package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/engine"
	"github.com/vedansh/vans/internal/store"
)

func TestADueTaskIsSentToTheWindowAndNotRun(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.AddSchedule(context.Background(), "tidy the desktop", `{"kind":"daily","minute":1020}`, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	srv := &Server{Engine: &engine.Engine{Schedules: st}}
	srv.out = json.NewEncoder(&out)

	if n := srv.bringUpWhatIsDue(context.Background(), time.Now()); n != 1 {
		t.Fatalf("brought up %d, want 1", n)
	}
	sent := out.String()
	if !strings.Contains(sent, `"method":"schedule_due"`) || !strings.Contains(sent, "tidy the desktop") {
		t.Errorf("sent %q, want the window told what is due", sent)
	}
	if n := srv.bringUpWhatIsDue(context.Background(), time.Now()); n != 0 {
		t.Errorf("brought it up again %d more time(s)", n)
	}
}
