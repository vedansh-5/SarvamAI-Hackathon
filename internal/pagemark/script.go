package pagemark

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/vedansh/vans/internal/theme"
)

//go:embed mark.js
var markJS string

const HighlightName = "vans-mark"

type Result struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error"`
	Matched  int    `json:"matched"`
	Alike    int    `json:"alike"`
	Scrolled bool   `json:"scrolled"`
	Painted  bool   `json:"painted"`
}

func Script(m Match, t theme.Theme) (string, error) {
	params, err := json.Marshal(map[string]string{
		"how": string(m.How), "text": m.Text, "name": HighlightName,
		"soft": t.Soft, "ink": t.Ink,
	})
	if err != nil {
		return "", fmt.Errorf("pagemark: %w", err)
	}
	return markJS + "(" + string(params) + ");", nil
}

const ClearJS = `if (window.__vansMarks) { try { window.__vansMarks.off(); removed = true; } catch (e) {} }
    try { if (window.CSS && CSS.highlights && CSS.highlights.has("` + HighlightName + `")) { CSS.highlights.delete("` + HighlightName + `"); removed = true; } } catch (e) {}`
