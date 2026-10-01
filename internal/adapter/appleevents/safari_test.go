package appleevents

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type scripted struct {
	reply string
	err   error
	ran   []string
}

func (s *scripted) Run(_ context.Context, script string) ([]byte, error) {
	s.ran = append(s.ran, script)
	return []byte(s.reply), s.err
}

func TestSafariReadsThePageInItsCurrentTab(t *testing.T) {
	r := &scripted{reply: `{"ok":true,"value":"{\"host\":\"apple.com\"}"}`}
	s := &Safari{Runner: r}

	var got struct{ Host string }
	loaded, err := s.ReadFront(context.Background(), "(function(){return '{}'})()", &got)
	if err != nil || !loaded || got.Host != "apple.com" {
		t.Fatalf("loaded=%v err=%v got=%+v", loaded, err, got)
	}
	if !strings.Contains(r.ran[0], "doJavaScript") || !strings.Contains(r.ran[0], "currentTab") {
		t.Errorf("script = %s", r.ran[0])
	}
}

func TestSafariSaysAPageIsStillLoading(t *testing.T) {
	s := &Safari{Runner: &scripted{reply: `{"ok":true,"value":"__vans_loading__"}`}}
	var got struct{}
	if loaded, err := s.ReadFront(context.Background(), "1", &got); err != nil || loaded {
		t.Errorf("loaded=%v err=%v, want it reported as not ready rather than as an error", loaded, err)
	}
}

func TestSafariWithScriptingOffSaysHowToTurnItOn(t *testing.T) {
	s := &Safari{Runner: &scripted{reply: `{"ok":false,"error":"You must enable 'Allow JavaScript from Apple Events' in the Developer section"}`}}
	err := s.AskFront(context.Background(), "1", nil)
	if !errors.Is(err, ErrSafariScriptingOff) || !strings.Contains(Fix(err), "Develop") {
		t.Errorf("err = %v, fix = %q", err, Fix(err))
	}
}
