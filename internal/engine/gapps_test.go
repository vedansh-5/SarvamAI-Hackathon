package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeGoogleTab struct {
	reply  any
	err    error
	asked  string
	called int
}

func (f *fakeGoogleTab) Front(context.Context) (string, string, error) { return "", "", nil }

func (f *fakeGoogleTab) AskFront(_ context.Context, script string, out any) error {
	f.called++
	f.asked = script
	if f.err != nil {
		return f.err
	}
	raw, _ := json.Marshal(f.reply)
	return json.Unmarshal(raw, out)
}

const drawnSheet = `<html><head><title>Budget - Google Sheets</title></head><body><canvas></canvas></body></html>`

func TestASheetInFrontIsReadFromItsExportNotItsCanvas(t *testing.T) {
	tab := &fakeGoogleTab{reply: map[string]any{"status": 200, "text": "Item,Cost\r\nRent,900\r\n"}}
	e := &Engine{}
	got := e.googleFileIn(context.Background(), tab, "https://docs.google.com/spreadsheets/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/edit#gid=5", drawnSheet)
	if !strings.Contains(got, "<td>Rent</td><td>900</td>") || !strings.Contains(got, "<h1>Budget</h1>") {
		t.Errorf("got %s", got)
	}
	if !strings.Contains(tab.asked, "export?format=csv") || !strings.Contains(tab.asked, "gid=5\"") {
		t.Errorf("asked for %s", tab.asked)
	}
}

func TestAnythingElseIsReadAsDrawn(t *testing.T) {
	e := &Engine{}
	for _, tc := range []struct {
		name string
		tab  *fakeGoogleTab
		url  string
	}{
		{"not google", &fakeGoogleTab{}, "https://example.com/spreadsheets/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/edit"},
		{"refused", &fakeGoogleTab{reply: map[string]any{"status": 403}}, "https://docs.google.com/document/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/edit"},
		{"broken", &fakeGoogleTab{err: errors.New("no tab")}, "https://docs.google.com/document/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/edit"},
	} {
		if got := e.googleFileIn(context.Background(), tc.tab, tc.url, drawnSheet); got != drawnSheet {
			t.Errorf("%s: replaced the page with %s", tc.name, got)
		}
	}
}

type frontOnly struct{}

func (frontOnly) Front(context.Context) (string, string, error) { return "", "", nil }

func TestABrowserThatCannotRunScriptsIsLeftAlone(t *testing.T) {
	e := &Engine{}
	if got := e.googleFileIn(context.Background(), frontOnly{}, "https://docs.google.com/document/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/edit", drawnSheet); got != drawnSheet {
		t.Errorf("got %s", got)
	}
}
