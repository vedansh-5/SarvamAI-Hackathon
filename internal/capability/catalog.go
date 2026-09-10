package capability

import (
	"fmt"
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

func (c *Catalog) Scriptable() []App {
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
			name := strings.TrimSuffix(e.Name(), ".app")
			if seen[strings.ToLower(name)] {
				continue
			}
			path := filepath.Join(root, e.Name())
			if _, err := sdef.Locate(path); err != nil {
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

func (c *Catalog) Find(name string) (App, error) {
	want := normalise(name)
	if want == "" {
		return App{}, &UnknownAppError{Name: name}
	}

	apps := c.Scriptable()

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
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".app")
	return strings.Join(strings.Fields(s), " ")
}

func (c *Catalog) Dictionary(a App) (sdef.Dictionary, error) {
	return sdef.Load(a.Path)
}
