//go:build integration

package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vedansh/vans/internal/synth"
)

func TestLiveAFileTaskWrittenByTheModelRunsAndIsPutBack(t *testing.T) {
	liveModel(t)
	backend, _ := synth.FromEnv()
	writer, ok := backend.(FileWriter)
	if !ok {
		t.Skip("the configured model cannot write file operations")
	}
	_, desk := aHome(t)
	e := &Engine{FileOps: writer}
	ctx := context.Background()

	plan, err := e.planFiles(ctx, Ask{Instruction: "move the screenshots on my Desktop into a folder called Screenshots"}, "")
	if err != nil {
		t.Fatalf("planFiles: %v", err)
	}
	if plan.Files == nil {
		t.Fatalf("no file operations came back: %+v", plan)
	}
	t.Logf("preview:\n%s", plan.Files.Preview)
	if _, err := e.Commit(ctx, plan); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	for _, name := range []string{"shot 1.png", "shot 2.png"} {
		if _, err := os.Stat(filepath.Join(desk, "Screenshots", name)); err != nil {
			t.Errorf("%s is not in Desktop/Screenshots", name)
		}
	}
	if _, err := os.Stat(filepath.Join(desk, "notes.txt")); err != nil {
		t.Error("notes.txt was moved, and it is not a screenshot")
	}

	if err := e.putItBack(ctx, howToUndo{Kind: undoByMovingFilesBack, Files: plan.Files.done}); err != nil {
		t.Fatalf("putItBack: %v", err)
	}
	for _, name := range []string{"shot 1.png", "shot 2.png", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(desk, name)); err != nil {
			t.Errorf("%s is not back on the Desktop", name)
		}
	}
	if _, err := os.Stat(filepath.Join(desk, "Screenshots")); err == nil {
		t.Error("the folder that was made is still there after putting it back")
	}
}
