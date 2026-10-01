package capability

import (
	"fmt"
	"github.com/vedansh/vans/internal/names"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vedansh/vans/internal/capability/sdef"
)

type App struct {
	Name string
	Path string
}

type Catalog struct {
	Roots []string
}

func DefaultRoots() []string {
	roots := []string{
		"/Applications",
		"/Applications/Utilities",
		"/System/Applications",
		"/System/Applications/Utilities",
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Applications"))
	}
	return roots
}

func New() *Catalog { return &Catalog{Roots: DefaultRoots()} }

// Scriptable is the applications that publish a scripting dictionary, which is
// what the dictionary tier can drive.
func (c *Catalog) Scriptable() []App {
	return c.walk(func(path string) bool {
		_, err := sdef.Locate(path)
		return err == nil
	})
}

// Installed is every application on the Mac, dictionary or not.
//
// A wider list than Scriptable and needed for a different question. Deciding
// where a task is carried out is not the same as deciding what can be driven by
// Apple Events: System Settings publishes almost no dictionary and is still the
// answer to most questions about how a Mac is configured. Filtering it out of
// that choice would leave the coach naming somewhere it can reach instead of
// somewhere the task is done.
func (c *Catalog) Installed() []App {
	return c.walk(func(string) bool { return true })
}

func (c *Catalog) walk(keep func(path string) bool) []App {
	seen := map[string]bool{}
	var apps []App

	for _, root := range c.Roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".app") {
				continue
			}
			name := names.Clean(strings.TrimSuffix(e.Name(), ".app"))
			if name == "" || seen[strings.ToLower(name)] {
				continue
			}
			path := filepath.Join(root, e.Name())
			if !keep(path) {
				continue
			}
			seen[strings.ToLower(name)] = true
			apps = append(apps, App{Name: name, Path: path})
		}
	}

	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
	return apps
}

type UnknownAppError struct {
	Name string
	Near []string
}

func (e *UnknownAppError) Error() string {
	msg := fmt.Sprintf("capability: no scriptable application named %q", e.Name)
	if len(e.Near) > 0 {
		msg += ": did you mean " + strings.Join(e.Near, ", ") + "?"
	}
	return msg
}

// Find resolves a name to an application that can be driven by Apple Events, so
// it searches only the ones with a dictionary. A name that exists but declares
// nothing is unknown *to this tier*, which is what the callers of this mean.
func (c *Catalog) Find(name string) (App, error) {
	return find(name, c.Scriptable())
}

// FindInstalled resolves a name to any application on the Mac.
//
// For deciding where a task belongs rather than what can be scripted. System
// Settings and most of what people actually use publish no dictionary, and
// resolving those through Find rejected them as unknown — a question about the
// wallpaper would come back naming whatever happened to be in front, because the
// right answer had been filtered out before it could be matched.
func (c *Catalog) FindInstalled(name string) (App, error) {
	return find(name, c.Installed())
}

func find(name string, apps []App) (App, error) {
	want := normalise(name)
	if want == "" {
		return App{}, &UnknownAppError{Name: name}
	}

	for _, a := range apps {
		if normalise(a.Name) == want {
			return a, nil
		}
	}

	var near []App
	for _, a := range apps {
		n := normalise(a.Name)
		if strings.HasPrefix(n, want) || strings.Contains(n, want) {
			near = append(near, a)
		}
	}
	if len(near) == 1 {
		return near[0], nil
	}

	suggest := make([]string, 0, len(near))
	for _, a := range near {
		suggest = append(suggest, a.Name)
	}
	return App{}, &UnknownAppError{Name: name, Near: suggest}
}

func normalise(s string) string {
	s = strings.ToLower(names.Clean(s))
	s = strings.TrimSuffix(s, ".app")
	return strings.Join(strings.Fields(s), " ")
}

func (c *Catalog) Dictionary(a App) (*sdef.Dictionary, error) {
	return sdef.Load(a.Path)
}
