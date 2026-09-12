package uiact

import (
	"encoding/json"
	"fmt"
	"strings"
)

func literal(v string) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("uiact: encode literal: %w", err)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
