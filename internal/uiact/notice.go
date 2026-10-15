package uiact

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/vedansh/vans/internal/names"
)

type Noticed struct {
	App   string
	Role  string
	Title string
}

type Layer struct {
	Role  string
	Title string
}

var noticeable = map[string]bool{
	"AXButton":             true,
	"AXMenuItem":           true,
	"AXMenuBarItem":        true,
	"AXCheckBox":           true,
	"AXRadioButton":        true,
	"AXPopUpButton":        true,
	"AXMenuButton":         true,
	"AXDisclosureTriangle": true,
}

var neverNoticed = regexp.MustCompile(`(?i)^(1password.*|bitwarden|keychain access|passwords|lastpass|dashlane|keeper.*|enpass|vans|loginwindow|securityagent)$`)

const longestNoticedTitle = 60

func Pick(app string, layers []Layer, inWeb bool) (Noticed, bool) {
	app = names.Clean(app)
	if app == "" || inWeb || neverNoticed.MatchString(app) {
		return Noticed{}, false
	}
	for i, l := range layers {
		if i > 2 {
			break
		}
		if !noticeable[l.Role] {
			continue
		}
		title := strings.Join(strings.Fields(names.Clean(l.Title)), " ")
		if title == "" || utf8.RuneCountInString(title) > longestNoticedTitle {
			return Noticed{}, false
		}
		return Noticed{App: app, Role: l.Role, Title: title}, true
	}
	return Noticed{}, false
}
