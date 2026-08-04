package sdef

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolver maps an xi:include href to a readable path. Production uses
// SystemResolver; tests redirect includes into testdata so the suite runs with
// no dependency on /System.
type Resolver func(href string) (string, error)

// SystemResolver strips the file://localhost prefix Apple uses in includes.
func SystemResolver(href string) (string, error) {
	p := strings.TrimPrefix(href, "file://localhost")
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("sdef: unsupported include href %q", href)
	}
	return p, nil
}

// TestdataResolver maps any include to a same-named file in dir.
func TestdataResolver(dir string) Resolver {
	return func(href string) (string, error) {
		return filepath.Join(dir, filepath.Base(href)), nil
	}
}

// ParseFile reads an .sdef file and splices in everything it xi:includes.
func ParseFile(path string, r Resolver) (*Dictionary, error) {
	return parseFile(path, r, map[string]bool{})
}

func parseFile(path string, r Resolver, seen map[string]bool) (*Dictionary, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if seen[abs] {
		return &Dictionary{}, nil // already merged — break the cycle
	}
	seen[abs] = true

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("sdef: read %s: %w", abs, err)
	}

	var d Dictionary
	if err := xml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("sdef: parse %s: %w", abs, err)
	}

	// Included suites go first, so the including dictionary's own definitions
	// win on conflict — Mail redefines delete, move, and duplicate.
	var merged []Suite
	for _, inc := range d.Includes {
		p, err := r(inc.Href)
		if err != nil {
			return nil, err
		}
		sub, err := parseFile(p, r, seen)
		if err != nil {
			return nil, fmt.Errorf("sdef: include %s: %w", inc.Href, err)
		}
		merged = append(merged, sub.Suites...)
	}
	d.Suites = append(merged, d.Suites...)
	d.Includes = nil
	return &d, nil
}

// Commands flattens every visible command across all suites.
func (d *Dictionary) Commands() []Command {
	var out []Command
	for _, s := range d.Suites {
		if isYes(s.Hidden) {
			continue
		}
		for _, c := range s.Commands {
			if isYes(c.Hidden) {
				continue
			}
			out = append(out, c)
		}
	}
	return out
}

// Classes flattens every visible class across all suites.
func (d *Dictionary) Classes() []Class {
	var out []Class
	for _, s := range d.Suites {
		if isYes(s.Hidden) {
			continue
		}
		for _, c := range s.Classes {
			if isYes(c.Hidden) {
				continue
			}
			out = append(out, c)
		}
	}
	return out
}

// VisibleProperties drops hidden properties.
func (c Class) VisibleProperties() []Property {
	var out []Property
	for _, p := range c.Properties {
		if isYes(p.Hidden) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// FindCommand returns the first visible command with the given name.
func (d *Dictionary) FindCommand(name string) (Command, bool) {
	for _, c := range d.Commands() {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}
