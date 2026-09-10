package sdef

import (
	"fmt"
	"path/filepath"
	"strings"
)

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

func Load(appPath string) (*Dictionary, error) {
	path, err := Locate(appPath)
	if err != nil {
		return nil, err
	}
	return ParseFile(path, SystemResolver)
}
