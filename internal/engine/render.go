package engine

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

// HTML renders a document as the body of a note.
//
// The title is the first line because Notes.app derives a note's name from it
// (Notes.sdef: name is normally the first line of the body). Getting the
// document to name itself is one fewer AppleEvent and one fewer thing to
// verify.
func (d Document) HTML() string {
	var b strings.Builder

	fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(d.Title))
	b.WriteString("<table>\n<tr>")
	for _, f := range d.Fields {
		fmt.Fprintf(&b, "<th>%s</th>", html.EscapeString(prettyField(f)))
	}
	b.WriteString("</tr>\n")

	for _, row := range d.Rows {
		b.WriteString("<tr>")
		for _, f := range d.Fields {
			fmt.Fprintf(&b, "<td>%s</td>", cell(row[f]))
		}
		b.WriteString("</tr>\n")
	}

	b.WriteString("</table>\n")
	return b.String()
}

// cell renders one value, linking it when it is safe to link.
//
// Only http and https become anchors. Extracted values are attacker-controlled
// in the ordinary case — any page can put whatever it likes in an href — and a
// javascript: or data: URL rendered as a clickable link in the user's notes is
// a page reaching out of the browser into an app that trusts it. Anything else
// is shown as text.
func cell(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return ""
	}

	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return html.EscapeString(v)
	}
	return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(trimmed), html.EscapeString(trimmed))
}

// prettyField turns a snake_case field name into a column heading.
func prettyField(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// Preview renders the document as plain text for the confirmation gate.
//
// This is what a person reads before approving a write, so it shows the data
// itself rather than a count. "Write 40 rows to Notes?" is not a question
// anyone can answer; the first few rows are.
func (d Document) Preview(maxRows int) string {
	if maxRows <= 0 {
		maxRows = 5
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", d.Title)

	widths := make([]int, len(d.Fields))
	for i, f := range d.Fields {
		widths[i] = len(prettyField(f))
	}
	shown := d.Rows
	if len(shown) > maxRows {
		shown = shown[:maxRows]
	}
	for _, row := range shown {
		for i, f := range d.Fields {
			if n := len(cellText(row[f], 48)); n > widths[i] {
				widths[i] = n
			}
		}
	}

	for i, f := range d.Fields {
		fmt.Fprintf(&b, "%-*s  ", widths[i], prettyField(f))
	}
	b.WriteString("\n")
	for i := range d.Fields {
		b.WriteString(strings.Repeat("-", widths[i]) + "  ")
	}
	b.WriteString("\n")

	for _, row := range shown {
		for i, f := range d.Fields {
			fmt.Fprintf(&b, "%-*s  ", widths[i], cellText(row[f], 48))
		}
		b.WriteString("\n")
	}

	if len(d.Rows) > len(shown) {
		fmt.Fprintf(&b, "\n… and %d more rows (%d total)\n", len(d.Rows)-len(shown), len(d.Rows))
	}
	return b.String()
}

func cellText(s string, width int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= width {
		return s
	}
	return string([]rune(s)[:width-1]) + "…"
}
