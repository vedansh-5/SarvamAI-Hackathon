package names

import (
	"strings"
	"unicode"
)

func Clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func Same(a, b string) bool {
	return strings.EqualFold(Clean(a), Clean(b))
}

func Strip(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
}

func Plain(s string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(Strip(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if gap && b.Len() > 0 {
				b.WriteByte(' ')
			}
			gap = false
			b.WriteRune(r)
			continue
		}
		gap = true
	}
	return b.String()
}
