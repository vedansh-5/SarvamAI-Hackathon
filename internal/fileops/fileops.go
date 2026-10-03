package fileops

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	Move   = "move"
	Rename = "rename"
	Copy   = "copy"
	Trash  = "trash"
	Mkdir  = "mkdir"
)

const (
	MostEntries = 400
	MostOps     = 200
)

type Op struct {
	Do   string `json:"op"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type Entry struct {
	Name     string
	Folder   bool
	Size     int64
	Modified time.Time
}

type Folder struct {
	Path    string
	Entries []Entry
	LeftOut int
}

type InvalidError struct {
	Reasons []string
}

func (e *InvalidError) Error() string {
	return "fileops: these operations will not be run: " + strings.Join(e.Reasons, "; ")
}

func Home() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Clean(home)
}

func Within(path, root string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func hidden(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") && part != "." {
			return true
		}
	}
	return false
}

func Allowed(path, home string) string {
	path = filepath.Clean(path)
	switch {
	case !filepath.IsAbs(path):
		return fmt.Sprintf("%s is not a full path", path)
	case !Within(path, home):
		return fmt.Sprintf("%s is outside your home folder, which is as far as VANS reaches", path)
	case path == home:
		return "the home folder itself is not something to move or rename"
	case hidden(path, home):
		return fmt.Sprintf("%s is hidden, and hidden files are left alone", path)
	case Within(path, filepath.Join(home, "Library")):
		return fmt.Sprintf("%s is inside Library, which applications own", path)
	}
	return ""
}

func List(path string) (Folder, error) {
	path = filepath.Clean(path)
	items, err := os.ReadDir(path)
	if err != nil {
		return Folder{}, err
	}
	out := Folder{Path: path}
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}
		if len(out.Entries) == MostEntries {
			out.LeftOut++
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		out.Entries = append(out.Entries, Entry{
			Name: item.Name(), Folder: item.IsDir(), Size: info.Size(), Modified: info.ModTime(),
		})
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		return strings.ToLower(out.Entries[i].Name) < strings.ToLower(out.Entries[j].Name)
	})
	return out, nil
}

func Validate(ops []Op, shown []Folder, home string) error {
	var reasons []string
	add := func(i int, format string, args ...any) {
		reasons = append(reasons, fmt.Sprintf("operation %d: ", i+1)+fmt.Sprintf(format, args...))
	}
	if len(ops) == 0 {
		return &InvalidError{Reasons: []string{"there are no operations"}}
	}
	if len(ops) > MostOps {
		return &InvalidError{Reasons: []string{fmt.Sprintf("%d operations, and %d is the most one request may make", len(ops), MostOps)}}
	}

	exists := map[string]bool{}
	folders := map[string]bool{}
	for _, f := range shown {
		folders[filepath.Clean(f.Path)] = true
		for _, e := range f.Entries {
			full := filepath.Join(f.Path, e.Name)
			exists[full] = true
			if e.Folder {
				folders[full] = true
			}
		}
	}
	for i, op := range ops {
		from, to := filepath.Clean(op.From), filepath.Clean(op.To)
		needsFrom := op.Do != Mkdir
		needsTo := op.Do != Trash

		if needsFrom {
			if strings.TrimSpace(op.From) == "" {
				add(i, "it does not say what to %s", op.Do)
				continue
			}
			if !exists[from] {
				add(i, "%s is not one of the items in the folders that were named", from)
				continue
			}
			if why := Allowed(from, home); why != "" {
				add(i, "%s", why)
				continue
			}
		}
		if needsTo {
			if strings.TrimSpace(op.To) == "" {
				add(i, "it does not say where to")
				continue
			}
			if why := Allowed(to, home); why != "" {
				add(i, "%s", why)
				continue
			}
			if exists[to] {
				add(i, "%s is already there, and nothing is overwritten", to)
				continue
			}
			if !folders[filepath.Dir(to)] {
				add(i, "%s is not a folder that was named or made earlier in the list", filepath.Dir(to))
				continue
			}
		}

		switch op.Do {
		case Rename:
			if filepath.Dir(from) != filepath.Dir(to) {
				add(i, "a rename stays in the same folder; going elsewhere is a move")
				continue
			}
			delete(exists, from)
			exists[to] = true
			if folders[from] {
				delete(folders, from)
				folders[to] = true
			}
		case Move:
			if Within(to, from) {
				add(i, "%s cannot be moved inside itself", from)
				continue
			}
			delete(exists, from)
			exists[to] = true
			if folders[from] {
				delete(folders, from)
				folders[to] = true
			}
		case Copy:
			if folders[from] {
				add(i, "%s is a folder, and only files are copied", from)
				continue
			}
			exists[to] = true
		case Trash:
			delete(exists, from)
			delete(folders, from)
		case Mkdir:
			exists[to] = true
			folders[to] = true
		default:
			add(i, "%q is not something VANS does to a file", op.Do)
		}
	}

	if len(reasons) == 0 {
		return nil
	}
	return &InvalidError{Reasons: reasons}
}

type Done struct {
	Do   string `json:"op"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

var Now = time.Now

func trashPath(home, from string) string {
	bin := filepath.Join(home, ".Trash")
	name := filepath.Base(from)
	target := filepath.Join(bin, name)
	if _, err := os.Lstat(target); err != nil {
		return target
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	return filepath.Join(bin, fmt.Sprintf("%s %s%s", stem, Now().Format("15.04.05.000"), ext))
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(to)
		return err
	}
	return out.Close()
}

func free(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%s is already there, and nothing is overwritten", path)
	}
	return nil
}

func Apply(ops []Op, home string) ([]Done, error) {
	var done []Done
	for i, op := range ops {
		from, to := filepath.Clean(op.From), filepath.Clean(op.To)
		var err error
		record := Done{Do: op.Do, From: from, To: to}

		switch op.Do {
		case Move, Rename:
			if err = free(to); err == nil {
				err = os.Rename(from, to)
			}
		case Copy:
			if err = free(to); err == nil {
				err = copyFile(from, to)
			}
		case Mkdir:
			record.From = ""
			err = os.Mkdir(to, 0o755)
		case Trash:
			record.To = trashPath(home, from)
			if err = os.MkdirAll(filepath.Dir(record.To), 0o700); err == nil {
				err = os.Rename(from, record.To)
			}
		default:
			err = fmt.Errorf("%q is not something VANS does to a file", op.Do)
		}

		if err != nil {
			return done, fmt.Errorf("fileops: operation %d of %d (%s %s) failed, and the ones after it were not run: %w",
				i+1, len(ops), op.Do, filepath.Base(firstOf(from, to)), err)
		}
		done = append(done, record)
	}
	return done, nil
}

func firstOf(a, b string) string {
	if a != "" && a != "." {
		return a
	}
	return b
}

func Reverse(done []Done) error {
	for i := len(done) - 1; i >= 0; i-- {
		d := done[i]
		var err error
		switch d.Do {
		case Move, Rename, Trash:
			if err = free(d.From); err == nil {
				err = os.Rename(d.To, d.From)
			}
		case Copy:
			err = os.Remove(d.To)
		case Mkdir:
			err = os.Remove(d.To)
		}
		if err != nil {
			return fmt.Errorf("fileops: could not put %s back: %w", filepath.Base(firstOf(d.From, d.To)), err)
		}
	}
	return nil
}

func Landed(done []Done) bool {
	there := map[string]bool{}
	carry := func(from, to string) {
		for path, is := range there {
			if path != from && Within(path, from) {
				rel, _ := filepath.Rel(from, path)
				delete(there, path)
				there[filepath.Join(to, rel)] = is
			}
		}
	}
	for _, d := range done {
		switch d.Do {
		case Move, Rename, Trash:
			carry(d.From, d.To)
			there[d.From] = false
			there[d.To] = true
		case Copy, Mkdir:
			there[d.To] = true
		}
	}
	for path, want := range there {
		_, err := os.Lstat(path)
		if (err == nil) != want {
			return false
		}
	}
	return len(done) > 0
}
