package theme

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

//go:embed themes.json
var raw []byte

type Theme struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Accent   string `json:"accent"`
	OnAccent string `json:"onAccent"`
	Soft     string `json:"soft"`
	Ink      string `json:"ink"`
}

var (
	all      []Theme
	fallback string
)

var hex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func init() {
	var file struct {
		Default string  `json:"default"`
		Themes  []Theme `json:"themes"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		panic(fmt.Sprintf("theme: themes.json: %v", err))
	}
	for _, t := range file.Themes {
		for _, c := range []string{t.Accent, t.OnAccent, t.Soft, t.Ink} {
			if !hex.MatchString(c) {
				panic(fmt.Sprintf("theme: %s has %q, which is not a #rrggbb colour", t.ID, c))
			}
		}
	}
	all, fallback = file.Themes, file.Default
}

func All() []Theme { return append([]Theme(nil), all...) }

func Default() Theme { return Named(fallback) }

func Named(id string) Theme {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, t := range all {
		if t.ID == id {
			return t
		}
	}
	for _, t := range all {
		if t.ID == fallback {
			return t
		}
	}
	return all[0]
}

func Known(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, t := range all {
		if t.ID == id {
			return true
		}
	}
	return false
}

func RGB(color string) (r, g, b int) {
	n, _ := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 32)
	return int(n >> 16 & 255), int(n >> 8 & 255), int(n & 255)
}

func Alpha(color string, a float64) string {
	r, g, b := RGB(color)
	return fmt.Sprintf("rgba(%d,%d,%d,%.2f)", r, g, b, a)
}
