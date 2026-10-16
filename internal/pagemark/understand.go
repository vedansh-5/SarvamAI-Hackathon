package pagemark

import (
	"regexp"
	"strings"
)

type How string

const (
	StartsWith How = "start"
	EndsWith   How = "end"
	Contains   How = "contain"
	Equals     How = "equal"
)

type Match struct {
	How  How
	Text string
}

const verb = `(?:highlight|mark|tint|colou?r|find|spot|point\s+out)`

var (
	byPhrase = regexp.MustCompile(`(?i)^\s*(?:please\s+)?(?:(?:can|could|would)\s+you\s+)?` + verb + `\b(.*?)\b(start(?:s|ing)?\s+with|begin(?:s|ning)?\s+with|end(?:s|ing)?\s+(?:with|in)|contain(?:s|ing)?|includ(?:e|es|ing)|(?:that\s+)?say(?:s|ing)?|equal(?:s|\s+to)?|exactly|called|named)\s+(.+?)\s*$`)
	quoted   = regexp.MustCompile(`(?i)^\s*(?:please\s+)?` + verb + `\s+(?:the\s+)?(?:words?\s+|text\s+|numbers?\s+|values?\s+)?["“'‘](.+?)["”'’]\s*(?:on\s+(?:this|the)\s+page|here)?[.!?]?\s*$`)
	bare     = regexp.MustCompile(`(?i)^\s*(?:please\s+)?` + verb + `\s+(\S*\d\S*)\s*(?:on\s+(?:this|the)\s+page|here)?[.!?]?\s*$`)
	byWhom   = regexp.MustCompile(`(?i)^\s*(?:please\s+)?(?:(?:can|could|would)\s+you\s+)?` + verb + `\b(.*?)\b(?:(?:made|opened|created|written|authored|posted|submitted|filed|raised|sent|added|started)\s+)?by\s+(@?\S+?)\s*(?:(?:on|in)\s+(?:this|the)\s+(?:page|list|table|tab|screen)|here|please|for\s+me)?[.!?]?\s*$`)
	trailing = regexp.MustCompile(`(?i)[\s,]*(?:\b(?:on|in)\s+(?:this|the)\s+(?:page|list|table|tab|screen)\b|\bhere\b|\bplease\b|\bfor\s+me\b|\bin\s+(?:it|them)\b)[\s.!?]*$`)
	quotes   = strings.NewReplacer("“", "", "”", "", "‘", "", "’", "")
)

const MostText = 60

func Understand(instruction string) (Match, bool) {
	if m := quoted.FindStringSubmatch(instruction); m != nil {
		return checked(Match{How: Contains, Text: m[1]})
	}
	if m := bare.FindStringSubmatch(instruction); m != nil {
		return checked(Match{How: Contains, Text: m[1]})
	}
	m := byPhrase.FindStringSubmatch(instruction)
	if m == nil {
		return byAuthor(instruction)
	}
	if strings.Contains(strings.ToLower(m[1]), " how ") {
		return Match{}, false
	}
	phrase := strings.ToLower(m[2])
	how := Contains
	switch {
	case strings.HasPrefix(phrase, "start") || strings.HasPrefix(phrase, "begin"):
		how = StartsWith
	case strings.HasPrefix(phrase, "end"):
		how = EndsWith
	case strings.HasPrefix(phrase, "equal") || phrase == "exactly" || phrase == "called" || phrase == "named":
		how = Equals
	}
	return checked(Match{How: how, Text: m[3]})
}

func byAuthor(instruction string) (Match, bool) {
	m := byWhom.FindStringSubmatch(instruction)
	if m == nil || strings.Contains(strings.ToLower(" "+m[1]+" "), " how ") || !looksLikeAName(m[2]) {
		return Match{}, false
	}
	return checked(Match{How: Contains, Text: strings.TrimPrefix(m[2], "@")})
}

func looksLikeAName(word string) bool {
	if strings.HasPrefix(word, "@") {
		return true
	}
	for i, r := range word {
		if (i == 0 && r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-_.", r) {
			return true
		}
	}
	return false
}

func checked(m Match) (Match, bool) {
	text := m.Text
	for {
		cut := trailing.ReplaceAllString(text, "")
		if cut == text {
			break
		}
		text = cut
	}
	text = strings.TrimSpace(quotes.Replace(text))
	text = strings.Trim(text, `"'.!?,;: `)
	if text == "" || len([]rune(text)) > MostText {
		return Match{}, false
	}
	m.Text = text
	return m, true
}

func (m Match) Says() string {
	switch m.How {
	case StartsWith:
		return "start with " + m.Text
	case EndsWith:
		return "end with " + m.Text
	case Equals:
		return "are " + m.Text
	default:
		return "contain " + m.Text
	}
}
