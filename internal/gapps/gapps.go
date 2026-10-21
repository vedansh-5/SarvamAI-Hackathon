package gapps

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

const (
	Document     = "document"
	Spreadsheet  = "spreadsheet"
	Presentation = "presentation"

	MostBytes = 2 << 20
	MostRows  = 2000
	MostCols  = 60
)

var (
	idPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{20,}$`)
	gidPattern = regexp.MustCompile(`(?:^|[#&?])gid=([0-9]+)`)
	kinds      = map[string]string{"document": Document, "spreadsheets": Spreadsheet, "presentation": Presentation}
	suffixes   = []string{" - Google Docs", " - Google Sheets", " - Google Slides"}
)

type File struct {
	Kind string
	ID   string
	Gid  string
}

func Recognise(pageURL string) (File, bool) {
	u, err := url.Parse(pageURL)
	if err != nil || u.Host != "docs.google.com" {
		return File{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "a" {
		parts = parts[2:]
	}
	if len(parts) == 0 {
		return File{}, false
	}
	kind, ok := kinds[parts[0]]
	rest := parts[1:]
	if len(rest) >= 2 && rest[0] == "u" {
		rest = rest[2:]
	}
	if !ok || len(rest) < 2 || rest[0] != "d" || !idPattern.MatchString(rest[1]) {
		return File{}, false
	}
	f := File{Kind: kind, ID: rest[1]}
	if kind == Spreadsheet {
		f.Gid = "0"
		for _, where := range []string{u.Fragment, u.RawQuery} {
			if m := gidPattern.FindStringSubmatch(where); m != nil {
				f.Gid = m[1]
				break
			}
		}
	}
	return f, true
}

func (f File) ExportPath() string {
	switch f.Kind {
	case Document:
		return "/document/d/" + f.ID + "/export?format=txt"
	case Spreadsheet:
		return "/spreadsheets/d/" + f.ID + "/export?format=csv&gid=" + f.Gid
	case Presentation:
		return "/presentation/d/" + f.ID + "/export/txt"
	}
	return ""
}

func (f File) ReadScript() (string, error) {
	path := f.ExportPath()
	if path == "" || !idPattern.MatchString(f.ID) {
		return "", fmt.Errorf("gapps: %q is not a file VANS can read", f.ID)
	}
	literal, err := json.Marshal(path)
	if err != nil {
		return "", err
	}
	return `(function () {
  try {
    var x = new XMLHttpRequest();
    x.open("GET", ` + string(literal) + `, false);
    x.send();
    var text = x.responseText || "";
    return JSON.stringify({ status: x.status, text: text.slice(0, ` + fmt.Sprint(MostBytes) + `), cut: text.length > ` + fmt.Sprint(MostBytes) + ` });
  } catch (e) {
    return JSON.stringify({ status: 0, text: "", error: String(e) });
  }
})()`, nil
}

type Export struct {
	Status int    `json:"status"`
	Text   string `json:"text"`
	Cut    bool   `json:"cut"`
	Error  string `json:"error"`
}

func (e Export) Err() error {
	switch {
	case e.Error != "":
		return fmt.Errorf("gapps: the page could not fetch its own export: %s", e.Error)
	case e.Status == 401 || e.Status == 403:
		return fmt.Errorf("gapps: this account cannot download the file (status %d)", e.Status)
	case e.Status != 200:
		return fmt.Errorf("gapps: the export answered %d", e.Status)
	case strings.HasPrefix(strings.TrimSpace(strings.ToLower(e.Text)), "<!doctype html"):
		return fmt.Errorf("gapps: the export came back as a web page, not the file")
	}
	return nil
}

func Title(pageHTML string) string {
	lower := strings.ToLower(pageHTML)
	start := strings.Index(lower, "<title>")
	if start < 0 {
		return ""
	}
	start += len("<title>")
	end := strings.Index(lower[start:], "</title>")
	if end < 0 {
		return ""
	}
	title := strings.TrimSpace(html.UnescapeString(pageHTML[start : start+end]))
	for _, s := range suffixes {
		title = strings.TrimSuffix(title, s)
	}
	return strings.TrimSpace(title)
}

func Page(f File, title, text string) (string, error) {
	var body strings.Builder
	switch f.Kind {
	case Spreadsheet:
		table, err := tableOf(text)
		if err != nil {
			return "", err
		}
		body.WriteString(table)
	default:
		for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "\uFEFF"))
			if line == "" {
				continue
			}
			body.WriteString("<p>" + html.EscapeString(line) + "</p>\n")
		}
	}
	name := html.EscapeString(title)
	return "<!doctype html><html><head><meta charset=\"utf-8\"><title>" + name + "</title></head><body><main><h1>" +
		name + "</h1>\n" + body.String() + "</main></body></html>", nil
}

func tableOf(text string) (string, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\uFEFF")))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return "", fmt.Errorf("gapps: read the sheet: %w", err)
	}
	var kept [][]string
	width := 0
	for _, row := range rows {
		if len(kept) >= MostRows+1 {
			break
		}
		last := -1
		for i, cell := range row {
			if strings.TrimSpace(cell) != "" {
				last = i
			}
		}
		if last < 0 {
			continue
		}
		if last+1 > width {
			width = last + 1
		}
		kept = append(kept, row)
	}
	if width > MostCols {
		width = MostCols
	}
	if len(kept) == 0 {
		return "<p>The sheet is empty.</p>", nil
	}
	var b strings.Builder
	b.WriteString("<table>\n<thead><tr>")
	for i := 0; i < width; i++ {
		b.WriteString("<th>" + html.EscapeString(cellAt(kept[0], i)) + "</th>")
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	for _, row := range kept[1:] {
		b.WriteString("<tr>")
		for i := 0; i < width; i++ {
			b.WriteString("<td>" + html.EscapeString(cellAt(row, i)) + "</td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>")
	return b.String(), nil
}

func cellAt(row []string, i int) string {
	if i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}
