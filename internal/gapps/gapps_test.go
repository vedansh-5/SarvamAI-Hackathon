package gapps

import (
	"strings"
	"testing"
)

const someID = "195j9eDD3ccgjQRttHhJPymLJUCOUjs-jmwTrekvdjFE"

func TestGoogleFilesAreKnownByTheirAddress(t *testing.T) {
	cases := []struct {
		url  string
		want File
		ok   bool
	}{
		{"https://docs.google.com/document/d/" + someID + "/edit", File{Kind: Document, ID: someID}, true},
		{"https://docs.google.com/document/u/1/d/" + someID + "/edit?tab=t.0", File{Kind: Document, ID: someID}, true},
		{"https://docs.google.com/spreadsheets/d/" + someID + "/edit#gid=1234", File{Kind: Spreadsheet, ID: someID, Gid: "1234"}, true},
		{"https://docs.google.com/spreadsheets/d/" + someID + "/edit?gid=77#gid=77", File{Kind: Spreadsheet, ID: someID, Gid: "77"}, true},
		{"https://docs.google.com/spreadsheets/d/" + someID + "/edit", File{Kind: Spreadsheet, ID: someID, Gid: "0"}, true},
		{"https://docs.google.com/presentation/d/" + someID + "/edit#slide=id.p", File{Kind: Presentation, ID: someID}, true},
		{"https://docs.google.com/a/example.com/document/d/" + someID + "/edit", File{Kind: Document, ID: someID}, true},
		{"https://docs.google.com/forms/d/" + someID + "/edit", File{}, false},
		{"https://docs.google.com/document/d/short/edit", File{}, false},
		{"https://docs.google.com/document/d/" + someID + "%22);alert(1);(/edit", File{}, false},
		{"https://evil.example/document/d/" + someID + "/edit", File{}, false},
		{"https://mail.google.com/mail/u/0/", File{}, false},
		{"not a url", File{}, false},
	}
	for _, c := range cases {
		got, ok := Recognise(c.url)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got %+v %v, want %+v %v", c.url, got, ok, c.want, c.ok)
		}
	}
}

func TestEachKindIsReadFromItsOwnExport(t *testing.T) {
	if got := (File{Kind: Document, ID: someID}).ExportPath(); got != "/document/d/"+someID+"/export?format=txt" {
		t.Error(got)
	}
	if got := (File{Kind: Spreadsheet, ID: someID, Gid: "9"}).ExportPath(); got != "/spreadsheets/d/"+someID+"/export?format=csv&gid=9" {
		t.Error(got)
	}
	if got := (File{Kind: Presentation, ID: someID}).ExportPath(); got != "/presentation/d/"+someID+"/export/txt" {
		t.Error(got)
	}
	if _, err := (File{Kind: Document, ID: "x\"y"}).ReadScript(); err == nil {
		t.Error("a made-up id should not become a script")
	}
	script, err := (File{Kind: Document, ID: someID}).ReadScript()
	if err != nil || !strings.Contains(script, `"/document/d/`+someID+`/export?format=txt"`) {
		t.Errorf("%v\n%s", err, script)
	}
}

func TestAnExportThatIsNotTheFileIsRefused(t *testing.T) {
	for _, e := range []Export{
		{Status: 403},
		{Status: 0, Error: "NetworkError"},
		{Status: 500},
		{Status: 200, Text: "<!DOCTYPE html><html>Sign in</html>"},
	} {
		if e.Err() == nil {
			t.Errorf("%+v was taken as the file", e)
		}
	}
	if err := (Export{Status: 200, Text: "Hello"}).Err(); err != nil {
		t.Error(err)
	}
}

func TestTheTitleLosesTheAppName(t *testing.T) {
	if got := Title("<html><head><title>Q3 plan &amp; budget - Google Sheets</title>"); got != "Q3 plan & budget" {
		t.Error(got)
	}
	if got := Title("<html>"); got != "" {
		t.Error(got)
	}
}

func TestADocumentBecomesParagraphs(t *testing.T) {
	page, err := Page(File{Kind: Document}, "Notes", "\uFEFFFirst line\r\n\r\n<b>not bold</b>\nLast")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<title>Notes</title>", "<h1>Notes</h1>", "<p>First line</p>", "<p>&lt;b&gt;not bold&lt;/b&gt;</p>", "<p>Last</p>"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %s in\n%s", want, page)
		}
	}
	if strings.Contains(page, "<p></p>") {
		t.Error("blank lines became empty paragraphs")
	}
}

func TestASheetBecomesATableWithoutItsEmptyEdges(t *testing.T) {
	csv := "Name,Score,,\r\nAsha,9,,\r\n,,,\r\n\"Ben, Jr\",7,,\r\n"
	page, err := Page(File{Kind: Spreadsheet}, "Scores", csv)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<th>Name</th><th>Score</th></tr>", "<td>Asha</td><td>9</td>", "<td>Ben, Jr</td><td>7</td>"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %s in\n%s", want, page)
		}
	}
	if strings.Count(page, "<tr>") != 3 {
		t.Errorf("an empty row was kept:\n%s", page)
	}
	if empty, _ := Page(File{Kind: Spreadsheet}, "x", ",,\n,,\n"); !strings.Contains(empty, "The sheet is empty.") {
		t.Error(empty)
	}
}
