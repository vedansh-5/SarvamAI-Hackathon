package sdef

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Resolver maps an xi:include href to a readable path. Production uses
// SystemResolver; tests redirect includes into testdata so the suite runs with
// no dependency on /System.
type Resolver func(href string) (string, error)

// SystemResolver turns an include href into a path on this machine. Apple
// writes both file://localhost/... and file:///..., and a dictionary that
// tries to include something over the network is refused rather than fetched.
func SystemResolver(href string) (string, error) {
	p := strings.TrimPrefix(strings.TrimPrefix(href, "file://localhost"), "file://")
	if strings.Contains(href, "://") && !strings.HasPrefix(href, "file:") {
		return "", fmt.Errorf("sdef: refusing to fetch an include over the network: %q", href)
	}
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
		drop, err := excludedCommands(inc.Xpointer)
		if err != nil {
			return nil, fmt.Errorf("sdef: include %s: %w", inc.Href, err)
		}
		sub, err := parseFile(p, r, seen)
		if err != nil {
			return nil, fmt.Errorf("sdef: include %s: %w", inc.Href, err)
		}
		for _, s := range sub.Suites {
			s.Commands = withoutCommands(s.Commands, drop)
			merged = append(merged, s)
		}
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

// FindCommand returns the visible command with the given name, preferring the
// last definition of it.
//
// Included suites are spliced in first so that the standard commands read
// first, which means a redefinition arrives later — and Mail, Safari and
// others do redefine delete, move and duplicate. Taking the first match
// handed back the standard suite's version of a command the application had
// deliberately replaced.
func (d *Dictionary) FindCommand(name string) (Command, bool) {
	commands := d.Commands()
	for i := len(commands) - 1; i >= 0; i-- {
		if commands[i].Name == name {
			return commands[i], true
		}
	}
	return Command{}, false
}

// An app whose xpointer removes commands from the standard suite genuinely
// does not respond to them — Safari drops make, and others drop delete,
// duplicate or move. Claiming otherwise produces a plan that is confirmed and
// then fails at the Apple event. An xpointer in any other shape is an error
// rather than a guess.
const allSuites = "xpointer(/dictionary/suite)"

var (
	suiteMinusCommands = regexp.MustCompile(`^xpointer\(/dictionary/suite/node\(\)\[not\(self::command and .*\)\]\)$`)
	commandName        = regexp.MustCompile(`@name\s*=\s*'([^']*)'`)
)

func excludedCommands(xpointer string) (map[string]bool, error) {
	if xpointer == "" || xpointer == allSuites {
		return nil, nil
	}
	if !suiteMinusCommands.MatchString(xpointer) {
		return nil, fmt.Errorf("unsupported xpointer %q", xpointer)
	}

	names := commandName.FindAllStringSubmatch(xpointer, -1)
	if len(names) == 0 {
		return nil, fmt.Errorf("unsupported xpointer %q", xpointer)
	}

	drop := map[string]bool{}
	for _, m := range names {
		drop[strings.ToLower(m[1])] = true
	}
	return drop, nil
}

func withoutCommands(commands []Command, drop map[string]bool) []Command {
	if len(drop) == 0 {
		return commands
	}
	kept := make([]Command, 0, len(commands))
	for _, c := range commands {
		if !drop[strings.ToLower(c.Name)] {
			kept = append(kept, c)
		}
	}
	return kept
}
