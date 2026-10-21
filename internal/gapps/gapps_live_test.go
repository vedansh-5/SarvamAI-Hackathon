package gapps

import (
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/headless"
)

func TestPublicGoogleFilesAreReadFromInsideTheirOwnTab(t *testing.T) {
	if testing.Short() {
		t.Skip("reaches docs.google.com")
	}
	c := headless.Open(t)
	cases := []struct {
		url, want string
	}{
		{"https://docs.google.com/document/d/195j9eDD3ccgjQRttHhJPymLJUCOUjs-jmwTrekvdjFE/edit", "<p>Sample doc</p>"},
		{"https://docs.google.com/spreadsheets/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/edit#gid=0", "<th>Student Name</th>"},
		{"https://docs.google.com/presentation/d/1EAYk18WDjIG-zp_0vLm3CsfQh_i8eXc67Jo2O9C6Vuc/edit", "<p>"},
	}
	for _, tc := range cases {
		f, ok := Recognise(tc.url)
		if !ok {
			t.Fatalf("%s not recognised", tc.url)
		}
		if err := c.OpenInFront(c.Ctx, tc.url); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 80 && c.Eval("document.readyState") != "complete"; i++ {
			time.Sleep(250 * time.Millisecond)
		}
		script, err := f.ReadScript()
		if err != nil {
			t.Fatal(err)
		}
		var got Export
		if err := c.AskFront(c.Ctx, script, &got); err != nil {
			t.Fatal(err)
		}
		if err := got.Err(); err != nil {
			t.Fatalf("%s: %v", tc.url, err)
		}
		page, err := Page(f, Title(c.Eval("document.documentElement.outerHTML")), got.Text)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(page, tc.want) {
			t.Errorf("%s: %q not in\n%.600s", tc.url, tc.want, page)
		}
		t.Logf("%s: %d bytes of export, title %q", f.Kind, len(got.Text), Title(c.Eval("document.documentElement.outerHTML")))
	}
}
