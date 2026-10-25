package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/fileops"
	"github.com/vedansh/vans/internal/macset"
	"github.com/vedansh/vans/internal/synth"
)

type fileWriter struct {
	answers []synth.FilesAnswer
	asked   []synth.FilesRequest
}

func (w *fileWriter) WriteFileOps(_ context.Context, req synth.FilesRequest) (synth.FilesAnswer, error) {
	w.asked = append(w.asked, req)
	if len(w.asked) <= len(w.answers) {
		return w.answers[len(w.asked)-1], nil
	}
	return synth.FilesAnswer{Why: "nothing fits"}, nil
}

func aHome(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	desk := filepath.Join(root, "Desktop")
	if err := os.MkdirAll(desk, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"shot 1.png", "shot 2.png", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(desk, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	was := homeDir
	homeDir = func() string { return root }
	t.Cleanup(func() { homeDir = was })
	return root, desk
}

func tidy(desk string) []fileops.Op {
	shots := filepath.Join(desk, "Screenshots")
	return []fileops.Op{
		{Do: fileops.Mkdir, To: shots},
		{Do: fileops.Move, From: filepath.Join(desk, "shot 1.png"), To: filepath.Join(shots, "shot 1.png")},
		{Do: fileops.Move, From: filepath.Join(desk, "shot 2.png"), To: filepath.Join(shots, "shot 2.png")},
	}
}

func TestFilesAreMovedOnlyAfterConfirmingAndCanBePutBack(t *testing.T) {
	_, desk := aHome(t)
	w := &fileWriter{answers: []synth.FilesAnswer{{Ops: tidy(desk), Why: "Move the two screenshots into a new folder."}}}
	e := &Engine{FileOps: w}

	plan, err := e.planFiles(context.Background(), Ask{Instruction: "move the screenshots on my Desktop into a folder called Screenshots"}, "")
	if err != nil {
		t.Fatalf("planFiles: %v", err)
	}
	if !plan.NeedsCommit() {
		t.Fatal("moving files was not held for confirmation")
	}
	if _, err := os.Stat(filepath.Join(desk, "Screenshots")); err == nil {
		t.Fatal("a folder was made before anything was confirmed")
	}
	for _, want := range []string{"Make the folder Desktop/Screenshots", "Move shot 1.png to Desktop/Screenshots", "can be put back"} {
		if !strings.Contains(plan.Files.Preview, want) {
			t.Errorf("want %q in the preview:\n%s", want, plan.Files.Preview)
		}
	}
	if len(w.asked) != 1 || len(w.asked[0].Folders) != 1 || len(w.asked[0].Folders[0].Entries) != 3 {
		t.Fatalf("the model was shown %+v, want the three items on the Desktop", w.asked)
	}

	receipt, err := e.Commit(context.Background(), plan)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !receipt.Verified {
		t.Error("the files moved and the run was not verified")
	}
	if _, err := os.Stat(filepath.Join(desk, "Screenshots", "shot 2.png")); err != nil {
		t.Error("shot 2.png is not in the new folder")
	}

	how := undoFor(plan)
	if !strings.Contains(how, `"kind":"files"`) {
		t.Fatalf("undo = %q, want the moves recorded", how)
	}
	if err := e.putItBack(context.Background(), howToUndo{Kind: undoByMovingFilesBack, Files: plan.Files.done}); err != nil {
		t.Fatalf("putItBack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(desk, "shot 1.png")); err != nil {
		t.Error("shot 1.png was not put back on the Desktop")
	}
}

func TestOperationsTheChecksRefuseAreSentBackAndNeverRun(t *testing.T) {
	root, desk := aHome(t)
	bad := []fileops.Op{{Do: fileops.Move, From: filepath.Join(desk, "notes.txt"), To: "/tmp/notes.txt"}}
	good := []fileops.Op{{Do: fileops.Rename, From: filepath.Join(desk, "notes.txt"), To: filepath.Join(desk, "ideas.txt")}}
	w := &fileWriter{answers: []synth.FilesAnswer{{Ops: bad}, {Ops: good}}}
	e := &Engine{FileOps: w}

	plan, err := e.planFiles(context.Background(), Ask{Instruction: "rename notes.txt on the desktop to ideas.txt"}, "")
	if err != nil {
		t.Fatalf("planFiles: %v", err)
	}
	if len(w.asked) != 2 || len(w.asked[1].Failures) == 0 {
		t.Fatalf("asked %d time(s) with failures %v, want the refusal sent back", len(w.asked), w.asked[len(w.asked)-1].Failures)
	}
	if plan.Files == nil || plan.Files.Ops[0].Do != fileops.Rename {
		t.Fatalf("plan = %+v, want the rename", plan.Files)
	}
	if _, err := os.Stat(filepath.Join(root, "Desktop", "notes.txt")); err != nil {
		t.Error("the refused operation touched the file")
	}
}

func TestAFolderThatChangedSincePlanningStopsTheRun(t *testing.T) {
	_, desk := aHome(t)
	w := &fileWriter{answers: []synth.FilesAnswer{{Ops: tidy(desk)}}}
	e := &Engine{FileOps: w}

	plan, err := e.planFiles(context.Background(), Ask{Instruction: "tidy the screenshots on my desktop"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(desk, "shot 2.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Commit(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Errorf("err = %v, want the run refused because the folder changed", err)
	}
	if _, err := os.Stat(filepath.Join(desk, "Screenshots")); err == nil {
		t.Error("part of the plan ran against a folder that no longer matched it")
	}
}

func TestNoFolderNamedAsksWhichAndCallsNoModel(t *testing.T) {
	aHome(t)
	w := &fileWriter{}
	e := &Engine{FileOps: w}

	plan, err := e.planFiles(context.Background(), Ask{Instruction: "tidy up my files"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.asked) != 0 || !strings.Contains(plan.Say, "which folder") {
		t.Errorf("asked the model %d time(s), say = %q; want to be asked which folder", len(w.asked), plan.Say)
	}
}

func TestFoldersAreFoundInWhatWasSaid(t *testing.T) {
	home := "/Users/someone"
	for said, want := range map[string]string{
		"sort my Downloads by type":                  "/Users/someone/Downloads",
		"clean up the desktop":                       "/Users/someone/Desktop",
		"rename the files in ~/Projects/site/images": "/Users/someone/Projects/site/images",
		"tidy /Users/someone/Work.":                  "/Users/someone/Work",
	} {
		got := foldersNamedIn(home, strings.ReplaceAll(said, "~", home))
		if len(got) != 1 || got[0] != want {
			t.Errorf("%q: got %q, want %q", said, got, want)
		}
	}
	if got := foldersNamedIn(home, "move everything from Downloads to Documents"); len(got) != 2 {
		t.Errorf("got %q, want both folders", got)
	}
}

type fakeSettings struct {
	values map[string]string
	sets   []string
}

func (f *fakeSettings) Get(_ context.Context, setting string) (string, error) {
	return f.values[setting], nil
}

func (f *fakeSettings) Set(_ context.Context, setting, value string) error {
	f.sets = append(f.sets, setting+"="+value)
	f.values[setting] = value
	return nil
}

func TestASettingIsChangedOnlyAfterConfirmingAndCanBePutBack(t *testing.T) {
	mac := &fakeSettings{values: map[string]string{macset.Volume: "30", macset.Dark: "off"}}
	e := &Engine{Settings: mac}

	plan, err := e.planSetting(context.Background(), "turn the volume up", "volume", "up")
	if err != nil {
		t.Fatalf("planSetting: %v", err)
	}
	if len(mac.sets) != 0 || !plan.NeedsCommit() {
		t.Fatalf("sets = %q before confirming; needs commit = %v", mac.sets, plan.NeedsCommit())
	}
	if !strings.Contains(plan.Setting.Preview, "40%") || !strings.Contains(plan.Setting.Preview, "30%") {
		t.Errorf("preview = %q, want what it will be and what it is", plan.Setting.Preview)
	}

	receipt, err := e.Commit(context.Background(), plan)
	if err != nil || !receipt.Verified || mac.values[macset.Volume] != "40" {
		t.Fatalf("receipt = %+v, err = %v, volume = %s", receipt, err, mac.values[macset.Volume])
	}

	if err := e.putItBack(context.Background(), howToUndo{Kind: undoBySettingItBack, Setting: macset.Volume, Was: "30"}); err != nil {
		t.Fatal(err)
	}
	if mac.values[macset.Volume] != "30" {
		t.Errorf("volume = %s after undoing, want 30", mac.values[macset.Volume])
	}
}

func TestASettingAlreadyAsAskedChangesNothing(t *testing.T) {
	mac := &fakeSettings{values: map[string]string{macset.Dark: "on"}}
	e := &Engine{Settings: mac}

	plan, err := e.planSetting(context.Background(), "switch to dark mode", "dark mode", "dark")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Setting != nil || plan.NeedsCommit() || !strings.Contains(plan.Say, "already") {
		t.Errorf("plan = %+v, want to be told it is already dark", plan)
	}
	if plan.Panel != "dark" || !strings.Contains(plan.Say, "VANS is dark now too") {
		t.Errorf("panel = %q, say = %q: an already dark Mac should still turn VANS dark", plan.Panel, plan.Say)
	}
}

func TestChangingTheAppearanceTakesVANSWithIt(t *testing.T) {
	e := &Engine{Settings: &fakeSettings{values: map[string]string{macset.Dark: "on", macset.Volume: "30"}}}
	light, err := e.planSetting(context.Background(), "switch to light mode", "light mode", "light")
	if err != nil {
		t.Fatal(err)
	}
	if light.Setting == nil || light.Panel != "light" {
		t.Errorf("light mode = %+v, want the Mac changed and VANS to follow", light)
	}
	volume, err := e.planSetting(context.Background(), "set the volume to 50", "volume", "50")
	if err != nil {
		t.Fatal(err)
	}
	if volume.Panel != "" {
		t.Errorf("the volume changed VANS's colours: %q", volume.Panel)
	}
}

func TestASettingVANSDoesNotChangeIsDeclinedWithWhatItCan(t *testing.T) {
	e := &Engine{Settings: &fakeSettings{values: map[string]string{}}}
	_, err := e.planSetting(context.Background(), "turn on bluetooth", "bluetooth", "on")
	if err == nil || !strings.Contains(err.Error(), "volume") {
		t.Errorf("err = %v, want it declined with what can be changed", err)
	}
}

func TestAFileTaskAsksItsQuestionAndCarriesOnWithTheAnswer(t *testing.T) {
	_, desk := aHome(t)
	rename := []fileops.Op{{Do: fileops.Rename, From: filepath.Join(desk, "shot 1.png"), To: filepath.Join(desk, "cover.png")}}
	w := &fileWriter{answers: []synth.FilesAnswer{
		{Question: "Which screenshot: shot 1.png or shot 2.png?"},
		{Ops: rename, Why: "Rename shot 1.png to cover.png."},
	}}
	var asked []string
	e := &Engine{FileOps: w, Log: func(string, ...any) {}}
	e.Question = func(_ context.Context, q string, _ []string) (string, error) {
		asked = append(asked, q)
		return "the first one", nil
	}

	plan, err := e.planFiles(context.Background(), Ask{Instruction: "rename the screenshot on my desktop to cover.png"}, "")
	if err != nil {
		t.Fatalf("planFiles: %v", err)
	}
	if len(asked) != 1 || plan.Files == nil || len(plan.Files.Ops) != 1 {
		t.Fatalf("asked %q, plan = %+v; want one question and then the rename", asked, plan.Files)
	}
	if told := w.asked[1].Ask; !strings.Contains(told, "the first one") || !strings.Contains(told, "Which screenshot") {
		t.Errorf("the model was not given the answer: %q", told)
	}
}

func TestAnUnansweredFileQuestionChangesNothing(t *testing.T) {
	_, desk := aHome(t)
	w := &fileWriter{answers: []synth.FilesAnswer{{Question: "Which one?"}}}
	e := &Engine{FileOps: w, Log: func(string, ...any) {}}
	e.Question = func(context.Context, string, []string) (string, error) { return "", nil }

	plan, err := e.planFiles(context.Background(), Ask{Instruction: "rename the screenshot on my desktop"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files != nil || plan.SayTitle != "Stopped" || len(w.asked) != 1 {
		t.Errorf("plan = %+v after %d ask(s); want it stopped with nothing planned", plan, len(w.asked))
	}
	if _, err := os.Stat(filepath.Join(desk, "shot 1.png")); err != nil {
		t.Error("a file was touched")
	}
}

func TestAFileTaskStopsAskingAfterAFewQuestions(t *testing.T) {
	aHome(t)
	endless := make([]synth.FilesAnswer, 10)
	for i := range endless {
		endless[i] = synth.FilesAnswer{Question: "And which?"}
	}
	w := &fileWriter{answers: endless}
	questions := 0
	e := &Engine{FileOps: w, Log: func(string, ...any) {}}
	e.Question = func(context.Context, string, []string) (string, error) { questions++; return "that one", nil }

	plan, err := e.planFiles(context.Background(), Ask{Instruction: "tidy my desktop"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if questions != mostFileQuestions || plan.SayTitle != "One thing first" {
		t.Errorf("asked %d time(s), title %q; want it to give up asking after %d", questions, plan.SayTitle, mostFileQuestions)
	}
}
