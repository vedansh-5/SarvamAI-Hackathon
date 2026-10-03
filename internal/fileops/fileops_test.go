package fileops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func home(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	desk := filepath.Join(root, "Desktop")
	for _, dir := range []string{desk, filepath.Join(desk, "Old"), filepath.Join(root, "Library"), filepath.Join(root, ".ssh")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"shot 1.png", "shot 2.png", "notes.txt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(desk, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, desk
}

func listed(t *testing.T, path string) []Folder {
	t.Helper()
	f, err := List(path)
	if err != nil {
		t.Fatal(err)
	}
	return []Folder{f}
}

func TestAFolderIsListedWithoutItsHiddenFiles(t *testing.T) {
	_, desk := home(t)
	f := listed(t, desk)[0]
	var names []string
	for _, e := range f.Entries {
		names = append(names, e.Name)
	}
	if got := strings.Join(names, ","); got != "notes.txt,Old,shot 1.png,shot 2.png" {
		t.Errorf("listed %q, want the four visible items in name order", got)
	}
}

func TestTidyingAFolderIsAcceptedAndRuns(t *testing.T) {
	root, desk := home(t)
	shots := filepath.Join(desk, "Screenshots")
	ops := []Op{
		{Do: Mkdir, To: shots},
		{Do: Move, From: filepath.Join(desk, "shot 1.png"), To: filepath.Join(shots, "shot 1.png")},
		{Do: Move, From: filepath.Join(desk, "shot 2.png"), To: filepath.Join(shots, "shot 2.png")},
		{Do: Rename, From: filepath.Join(desk, "notes.txt"), To: filepath.Join(desk, "ideas.txt")},
		{Do: Copy, From: filepath.Join(desk, "ideas.txt"), To: filepath.Join(desk, "Old", "ideas.txt")},
		{Do: Trash, From: filepath.Join(desk, "ideas.txt")},
	}
	if err := Validate(ops, listed(t, desk), root); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	done, err := Apply(ops, root)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !Landed(done) {
		t.Error("the operations ran and were not found to have landed")
	}
	for _, want := range []string{filepath.Join(shots, "shot 1.png"), filepath.Join(desk, "Old", "ideas.txt"), filepath.Join(root, ".Trash", "ideas.txt")} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("%s is missing after the run", want)
		}
	}

	if err := Reverse(done); err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	for _, back := range []string{"shot 1.png", "shot 2.png", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(desk, back)); err != nil {
			t.Errorf("%s was not put back", back)
		}
	}
	if _, err := os.Stat(shots); err == nil {
		t.Error("the folder that was made is still there after undoing")
	}
	if _, err := os.Stat(filepath.Join(desk, "Old", "ideas.txt")); err == nil {
		t.Error("the copy is still there after undoing")
	}
}

func TestWhatWasNotShownCannotBeTouched(t *testing.T) {
	root, desk := home(t)
	shown := listed(t, desk)

	for name, tc := range map[string]struct {
		op   Op
		want string
	}{
		"a hidden file":       {Op{Do: Trash, From: filepath.Join(desk, ".hidden")}, "not one of the items"},
		"another folder":      {Op{Do: Trash, From: filepath.Join(root, ".ssh")}, "not one of the items"},
		"outside home":        {Op{Do: Move, From: filepath.Join(desk, "notes.txt"), To: "/tmp/notes.txt"}, "outside your home folder"},
		"into Library":        {Op{Do: Move, From: filepath.Join(desk, "notes.txt"), To: filepath.Join(root, "Library", "notes.txt")}, "Library"},
		"over something":      {Op{Do: Move, From: filepath.Join(desk, "shot 1.png"), To: filepath.Join(desk, "shot 2.png")}, "nothing is overwritten"},
		"to an unknown place": {Op{Do: Move, From: filepath.Join(desk, "notes.txt"), To: filepath.Join(root, "Nowhere", "notes.txt")}, "not a folder that was named"},
		"rename elsewhere":    {Op{Do: Rename, From: filepath.Join(desk, "notes.txt"), To: filepath.Join(desk, "Old", "notes.txt")}, "same folder"},
		"copy a folder":       {Op{Do: Copy, From: filepath.Join(desk, "Old"), To: filepath.Join(desk, "Older")}, "only files are copied"},
		"an unknown verb":     {Op{Do: "delete", From: filepath.Join(desk, "notes.txt"), To: filepath.Join(desk, "x")}, "not something VANS does"},
	} {
		err := Validate([]Op{tc.op}, shown, root)
		var invalid *InvalidError
		if !errors.As(err, &invalid) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it refused for %q", name, err, tc.want)
		}
	}
}

func TestTheSameItemCannotBeUsedTwice(t *testing.T) {
	root, desk := home(t)
	ops := []Op{
		{Do: Trash, From: filepath.Join(desk, "notes.txt")},
		{Do: Rename, From: filepath.Join(desk, "notes.txt"), To: filepath.Join(desk, "ideas.txt")},
	}
	if err := Validate(ops, listed(t, desk), root); err == nil {
		t.Error("an item that had already been binned was renamed afterwards")
	}
}

func TestAFailureStopsTheRunAndSaysWhatWasDone(t *testing.T) {
	root, desk := home(t)
	ops := []Op{
		{Do: Rename, From: filepath.Join(desk, "notes.txt"), To: filepath.Join(desk, "ideas.txt")},
		{Do: Rename, From: filepath.Join(desk, "gone.txt"), To: filepath.Join(desk, "there.txt")},
		{Do: Trash, From: filepath.Join(desk, "shot 1.png")},
	}
	done, err := Apply(ops, root)
	if err == nil || len(done) != 1 {
		t.Fatalf("done = %v, err = %v; want one done and the failure reported", done, err)
	}
	if _, statErr := os.Stat(filepath.Join(desk, "shot 1.png")); statErr != nil {
		t.Error("an operation after the failure was still run")
	}
}

func TestBinningTwoThingsOfTheSameNameKeepsBoth(t *testing.T) {
	root, desk := home(t)
	if err := os.MkdirAll(filepath.Join(root, ".Trash"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".Trash", "notes.txt"), []byte("older"), 0o644); err != nil {
		t.Fatal(err)
	}
	done, err := Apply([]Op{{Do: Trash, From: filepath.Join(desk, "notes.txt")}}, root)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if done[0].To == filepath.Join(root, ".Trash", "notes.txt") {
		t.Error("binning replaced what was already in the bin under that name")
	}
	older, _ := os.ReadFile(filepath.Join(root, ".Trash", "notes.txt"))
	if string(older) != "older" {
		t.Error("what was already in the bin was overwritten")
	}
}
