package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vedansh/vans/internal/capability"
)

func aMacWith(t *testing.T, apps ...string) *capability.Catalog {
	t.Helper()
	root := t.TempDir()
	for _, app := range apps {
		if err := os.MkdirAll(filepath.Join(root, app+".app"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &capability.Catalog{Roots: []string{root}}
}

func aMac(t *testing.T) *capability.Catalog {
	t.Helper()
	return aMacWith(t, "System Settings", "Notes", "Safari", "Finder", "Mail", "Calendar", "Photos", "Maps", "Antigravity IDE")
}
