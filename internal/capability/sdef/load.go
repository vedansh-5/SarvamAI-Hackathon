package sdef

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxIncludeDepth = 4

var ErrNoDictionary = fmt.Errorf("sdef: the application ships no scripting dictionary")

func Locate(appPath string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(appPath, "Contents", "Resources", "*.sdef"))
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", ErrNoDictionary
	case 1:
		return matches[0], nil
	}

	want := strings.TrimSuffix(filepath.Base(appPath), ".app")
	for _, m := range matches {
		if strings.EqualFold(strings.TrimSuffix(filepath.Base(m), ".sdef"), want) {
			return m, nil
		}
	}
	return matches[0], nil
}

func Load(appPath string) (Dictionary, error) {
	path, err := Locate(appPath)
	if err != nil {
		return Dictionary{}, err
	}
	return LoadFile(path)
}

func LoadFile(path string) (Dictionary, error) {
	d, err := readFile(path)
	if err != nil {
		return Dictionary{}, err
	}
	if err := d.resolve(path, 0); err != nil {
		return Dictionary{}, err
	}
	return d, nil
}

func readFile(path string) (Dictionary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Dictionary{}, fmt.Errorf("sdef: %w", err)
	}
	var d Dictionary
	if err := xml.Unmarshal(data, &d); err != nil {
		return Dictionary{}, fmt.Errorf("sdef: %s: %w", path, err)
	}
	return d, nil
}

func (d *Dictionary) resolve(from string, depth int) error {
	if len(d.Includes) == 0 {
		return nil
	}
	if depth >= maxIncludeDepth {
		return fmt.Errorf("sdef: %s: includes nested more than %d deep", from, maxIncludeDepth)
	}

	includes := d.Includes
	d.Includes = nil

	for _, inc := range includes {
		path, err := includePath(inc.Href, filepath.Dir(from))
		if err != nil {
			return fmt.Errorf("sdef: %s: %w", from, err)
		}

		drop, err := excludedCommands(inc.Xpointer)
		if err != nil {
			return fmt.Errorf("sdef: %s: %w", from, err)
		}

		included, err := readFile(path)
		if err != nil {
			return err
		}
		if err := included.resolve(path, depth+1); err != nil {
			return err
		}

		for _, s := range included.Suites {
			s.Commands = withoutCommands(s.Commands, drop)
			d.Suites = append(d.Suites, s)
		}
	}
	return nil
}

func includePath(href, base string) (string, error) {
	switch {
	case strings.HasPrefix(href, "file://localhost/"):
		return "/" + strings.TrimPrefix(href, "file://localhost/"), nil
	case strings.HasPrefix(href, "file:///"):
		return "/" + strings.TrimPrefix(href, "file:///"), nil
	case strings.Contains(href, "://"):
		return "", fmt.Errorf("refusing to fetch an include over the network: %s", href)
	case filepath.IsAbs(href):
		return href, nil
	default:
		return filepath.Join(base, href), nil
	}
}

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
		if drop[strings.ToLower(c.Name)] {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}
