package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vedansh/vans/internal/fileops"
	"github.com/vedansh/vans/internal/synth"
)

type FileWriter interface {
	WriteFileOps(ctx context.Context, req synth.FilesRequest) (synth.FilesAnswer, error)
}

type FilesPlan struct {
	Ops     []fileops.Op
	Folders []string
	Preview string

	done []fileops.Done
}

const (
	mostFoldersRead  = 3
	mostLinesPreview = 14
	whereFilesLive   = "Finder"
)

var homeDir = fileops.Home

func (e *Engine) canDoFiles() bool { return e.FileOps != nil }

var knownFolders = map[string]string{
	"desktop": "Desktop", "downloads": "Downloads", "download": "Downloads", "documents": "Documents",
	"pictures": "Pictures", "movies": "Movies", "music": "Music",
}

var (
	pathInText   = regexp.MustCompile(`(?:^|[\s"'\x{201C}])((?:~|/Users)/[^\s"'\x{201D}]+)`)
	wordInText   = regexp.MustCompile(`[\p{L}]+`)
	homeAsAWhole = regexp.MustCompile(`(?i)\bhome (folder|directory)\b`)
)

func foldersNamedIn(home string, texts ...string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(path string) {
		path = filepath.Clean(path)
		if !seen[path] && len(out) < mostFoldersRead {
			seen[path] = true
			out = append(out, path)
		}
	}
	for _, text := range texts {
		for _, m := range pathInText.FindAllStringSubmatch(text, -1) {
			add(strings.TrimRight(expandHome(m[1]), ".,;:"))
		}
		for _, word := range wordInText.FindAllString(strings.ToLower(text), -1) {
			if name, ok := knownFolders[word]; ok {
				add(filepath.Join(home, name))
			}
		}
		if homeAsAWhole.MatchString(text) {
			add(home)
		}
	}
	return out
}

func mayBeRead(folder, home string) string {
	if folder == home {
		return ""
	}
	return fileops.Allowed(folder, home)
}

func (e *Engine) planFiles(ctx context.Context, in Ask, slot string) (*Plan, error) {
	home := homeDir()
	if home == "" {
		return nil, fmt.Errorf("engine: could not find your home folder")
	}

	folders := foldersNamedIn(home, in.Instruction, slot)
	selected := e.filesSelectedIn(ctx, in.Context)
	for _, file := range selected {
		parent := filepath.Dir(filepath.Clean(file))
		known := false
		for _, f := range folders {
			known = known || f == parent
		}
		if !known && len(folders) < mostFoldersRead {
			folders = append(folders, parent)
		}
	}
	if len(folders) == 0 {
		return saying("Which folder?",
			"Say which folder you mean, like Desktop or Downloads, or select the files in Finder and say “these”."), nil
	}

	var shown []fileops.Folder
	for _, folder := range folders {
		if why := mayBeRead(folder, home); why != "" {
			return saying("Can’t work there", why+"."), nil
		}
		listing, err := fileops.List(folder)
		if err != nil {
			return saying("Can’t read that folder", fmt.Sprintf("%s could not be read: %v.", folder, err)), nil
		}
		shown = append(shown, listing)
	}

	ask := strings.TrimSpace(in.Instruction)
	if len(selected) > 0 {
		ask += "\n\n" + filesNote(selected)
	}

	req := synth.FilesRequest{Ask: ask, Folders: shown}
	calls := 0
	for attempt := 1; attempt <= e.maxSynthesis(); attempt++ {
		calls++
		answer, err := e.FileOps.WriteFileOps(ctx, req)
		if err != nil {
			return nil, err
		}
		if answer.Question != "" && len(answer.Ops) == 0 {
			plan := saying("One thing first", answer.Question+"\n\nAsk again with that in it.")
			plan.SynthesisCalls = calls
			return plan, nil
		}
		if len(answer.Ops) == 0 {
			plan := saying("Nothing to do", reasonOr(answer.Why, "nothing in that folder fits what was asked"))
			plan.SynthesisCalls = calls
			return plan, nil
		}

		err = fileops.Validate(answer.Ops, shown, home)
		if err == nil {
			return &Plan{
				Task:           Task{Intent: strings.TrimSpace(in.Instruction), Title: whereFilesLive},
				Destination:    whereFilesLive,
				SynthesisCalls: calls,
				Files: &FilesPlan{
					Ops: answer.Ops, Folders: folders,
					Preview: filesPreview(answer.Ops, answer.Why, home),
				},
			}, nil
		}
		var invalid *fileops.InvalidError
		if !errors.As(err, &invalid) {
			return nil, err
		}
		e.logf("file operations refused: %s", strings.Join(invalid.Reasons, "; "))
		req.Failures = invalid.Reasons
	}

	return nil, &UnroutableError{
		Instruction: in.Instruction,
		Why:         "no list of file operations survived being checked against what is in that folder",
		Can:         e.capabilities(),
	}
}

func short(path, home string) string {
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		if rel == "." {
			return "your home folder"
		}
		return rel
	}
	return path
}

func filesPreview(ops []fileops.Op, why, home string) string {
	var lines []string
	for _, op := range ops {
		from, to := filepath.Base(op.From), short(filepath.Clean(op.To), home)
		switch op.Do {
		case fileops.Mkdir:
			lines = append(lines, "Make the folder "+to)
		case fileops.Move:
			lines = append(lines, fmt.Sprintf("Move %s to %s", from, short(filepath.Dir(filepath.Clean(op.To)), home)))
		case fileops.Rename:
			lines = append(lines, fmt.Sprintf("Rename %s to %s", from, filepath.Base(op.To)))
		case fileops.Copy:
			lines = append(lines, fmt.Sprintf("Copy %s to %s", from, to))
		case fileops.Trash:
			lines = append(lines, fmt.Sprintf("Put %s in the Bin", from))
		}
	}

	var b strings.Builder
	if why = strings.TrimSpace(why); why != "" {
		b.WriteString(why + "\n\n")
	}
	for i, line := range lines {
		if i == mostLinesPreview {
			fmt.Fprintf(&b, "  … and %d more\n", len(lines)-mostLinesPreview)
			break
		}
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\nNothing is overwritten and nothing is deleted for good. It can be put back from History.")
	return b.String()
}

func (e *Engine) runFiles(ctx context.Context, plan *Plan) (Receipt, error) {
	f := plan.Files
	home := homeDir()

	var shown []fileops.Folder
	for _, folder := range f.Folders {
		listing, err := fileops.List(folder)
		if err != nil {
			return Receipt{}, fmt.Errorf("engine: %s could not be read again before acting: %w", folder, err)
		}
		shown = append(shown, listing)
	}
	if err := fileops.Validate(f.Ops, shown, home); err != nil {
		return Receipt{}, fmt.Errorf("engine: the folder changed since this was planned, so nothing was done: %w", err)
	}

	done, err := fileops.Apply(f.Ops, home)
	f.done = done
	did := make([]string, 0, len(done))
	for _, d := range done {
		did = append(did, fmt.Sprintf("%s %s", d.Do, filepath.Base(firstPath(d.From, d.To))))
	}
	if err != nil {
		return Receipt{Title: whereFilesLive, Did: did}, err
	}
	e.logf("ran %d file operation(s)", len(done))
	return Receipt{Title: whereFilesLive, Verified: fileops.Landed(done), Did: did}, nil
}

func firstPath(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
