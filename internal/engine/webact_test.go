package engine

import "testing"

func TestPageBelongsToTheApplicationNamed(t *testing.T) {
	cases := []struct {
		url  string
		app  string
		want bool
	}{
		{"https://web.whatsapp.com/", "WhatsApp", true},
		{"https://www.notion.so/workspace", "Notion", true},
		{"https://app.slack.com/client/T1", "Slack", true},
		{"https://web.telegram.org/k/", "Telegram", true},
		{"https://github.com/vedansh-5/VANS/pulls", "Spotify", false},
		{"https://github.com/vedansh-5/VANS/pulls", "WhatsApp", false},
		{"https://web.whatsapp.com/", "", false},
		{"", "WhatsApp", false},
	}

	for _, c := range cases {
		if got := pageBelongsTo(c.url, c.app); got != c.want {
			t.Errorf("pageBelongsTo(%q, %q) = %v, want %v", c.url, c.app, got, c.want)
		}
	}
}
