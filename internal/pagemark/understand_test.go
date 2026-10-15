package pagemark

import "testing"

func TestWhatToMarkIsUnderstoodWithoutAModel(t *testing.T) {
	for _, c := range []struct {
		said string
		want Match
	}{
		{"highlight the ones that start with 2321", Match{StartsWith, "2321"}},
		{"Highlight the ones that start with 23.", Match{StartsWith, "23"}},
		{"mark the roll numbers beginning with 2310 on this page", Match{StartsWith, "2310"}},
		{"please highlight IDs ending in 07", Match{EndsWith, "07"}},
		{"highlight everything containing Absent", Match{Contains, "Absent"}},
		{"highlight the rows that say “not paid”", Match{Contains, "not paid"}},
		{`highlight "Pending"`, Match{Contains, "Pending"}},
		{"highlight 23215555", Match{Contains, "23215555"}},
		{"find the ones called Riya here", Match{Equals, "Riya"}},
		{"could you highlight the ones that start with 2321 for me", Match{StartsWith, "2321"}},
	} {
		got, ok := Understand(c.said)
		if !ok || got != c.want {
			t.Errorf("%q = %+v, %v; want %+v", c.said, got, ok, c.want)
		}
	}
}

func TestOtherRequestsAreLeftToTheRouter(t *testing.T) {
	for _, said := range []string{
		"highlight the headings",
		"highlight the prices",
		"show me how to start a timer",
		"start with the inbox",
		"how do I highlight text that starts with a capital",
		"clear the highlighting",
		"mark this email as read",
	} {
		if got, ok := Understand(said); ok {
			t.Errorf("%q was taken as marking text: %+v", said, got)
		}
	}
}
