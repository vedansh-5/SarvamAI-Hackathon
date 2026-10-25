package uiact

import "testing"

func TestOnlyNamedControlsOutsideWebPagesAreNoticed(t *testing.T) {
	cases := []struct {
		name   string
		app    string
		layers []Layer
		web    bool
		want   string
	}{
		{"a button", "Mail", []Layer{{"AXButton", "Archive"}}, false, "Archive"},
		{"the text on a button", "Mail", []Layer{{"AXStaticText", "Archive"}, {"AXButton", "Archive"}}, false, "Archive"},
		{"a menu item", "Finder", []Layer{{"AXMenuItem", "New Folder"}}, false, "New Folder"},
		{"switching windows in Stage Manager", "WindowManager", []Layer{{"AXButton", "Telegram @ Vedansh"}}, false, ""},
		{"switching apps in the Dock", "Dock", []Layer{{"AXButton", "WhatsApp"}}, false, ""},
		{"a row carries content", "Mail", []Layer{{"AXRow", "Dinner on Friday"}}, false, ""},
		{"text carries content", "Notes", []Layer{{"AXTextArea", "my diary"}}, false, ""},
		{"inside a web page", "Google Chrome", []Layer{{"AXButton", "Pay now"}}, true, ""},
		{"a password manager", "1Password 7", []Layer{{"AXButton", "Copy"}}, false, ""},
		{"Vspk itself", "VANS", []Layer{{"AXButton", "Allow all"}}, false, ""},
		{"no name", "Mail", []Layer{{"AXButton", ""}}, false, ""},
		{"too far up", "Mail", []Layer{{"AXImage", ""}, {"AXGroup", ""}, {"AXGroup", ""}, {"AXButton", "Archive"}}, false, ""},
	}
	for _, c := range cases {
		got, ok := Pick(c.app, c.layers, c.web)
		if c.want == "" && ok {
			t.Errorf("%s: noticed %+v, want nothing", c.name, got)
		}
		if c.want != "" && (!ok || got.Title != c.want) {
			t.Errorf("%s: got %+v (%v), want %q", c.name, got, ok, c.want)
		}
	}
}
