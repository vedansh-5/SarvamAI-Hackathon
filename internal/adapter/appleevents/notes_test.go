package appleevents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeRunner struct {
	script string
	out    []byte
	err    error
}

func (f *fakeRunner) Run(_ context.Context, script string) ([]byte, error) {
	f.script = script
	return f.out, f.err
}

// Body text is user-controlled and lands inside a generated script, so the
// escaping is a correctness boundary, not a nicety.
func TestCreateEscapesBody(t *testing.T) {
	nasty := "he said \"hi\"\nand \\ then; app.quit()"
	f := &fakeRunner{out: []byte(`{"id":"x-1"}`)}

	if _, err := (Notes{Runner: f}).Create(context.Background(), nasty); err != nil {
		t.Fatalf("Create: %v", err)
	}

	want, _ := json.Marshal(nasty)
	if !strings.Contains(f.script, string(want)) {
		t.Errorf("script missing escaped literal.\ngot:\n%s", f.script)
	}
	if strings.Contains(f.script, "\nand \\ then; app.quit()") {
		t.Error("raw body leaked into script unescaped")
	}
}

func TestCreateRejectsEmptyID(t *testing.T) {
	f := &fakeRunner{out: []byte(`{}`)}
	if _, err := (Notes{Runner: f}).Create(context.Background(), "hi"); err == nil {
		t.Error("want error when Notes returns no id")
	}
}

func TestGetDecodes(t *testing.T) {
	f := &fakeRunner{out: []byte(`{"id":"x-1","name":"Title","plaintext":"Title\nbody"}`)}

	got, err := (Notes{Runner: f}).Get(context.Background(), "x-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "x-1" || got.Name != "Title" {
		t.Errorf("got %+v", got)
	}
}

func TestIsPermissionDenied(t *testing.T) {
	denied := &ScriptError{Stderr: "execution error: Not authorized to send Apple events to Notes. (-1743)"}
	if !IsPermissionDenied(denied) {
		t.Error("want true for -1743")
	}
	if IsPermissionDenied(&ScriptError{Stderr: "some other failure"}) {
		t.Error("want false for unrelated error")
	}
}
