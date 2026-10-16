package uiact

import (
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
)

var ErrCannotReadWords = errors.New("uiact: the words in the picture could not be read")

type Word struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

type seenLine struct {
	Text       string     `json:"t"`
	Confidence float64    `json:"c"`
	Words      []seenWord `json:"words"`
}

type seenWord struct {
	At  int     `json:"at"`
	Len int     `json:"len"`
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	W   float64 `json:"w"`
	H   float64 `json:"h"`
}

type seenPage struct {
	Width  float64    `json:"width"`
	Height float64    `json:"height"`
	Lines  []seenLine `json:"lines"`
}

const (
	leastConfidence = 0.3
	gapInLetters    = 1.6
)

func pieces(page seenPage) []Word {
	var out []Word
	for _, line := range page.Lines {
		if line.Confidence < leastConfidence || len(line.Words) == 0 {
			continue
		}
		units := utf16.Encode([]rune(line.Text))
		words := append([]seenWord(nil), line.Words...)
		sort.Slice(words, func(i, j int) bool { return words[i].X < words[j].X })

		start := 0
		for i := 1; i <= len(words); i++ {
			split := i == len(words)
			if !split {
				prev, next := words[i-1], words[i]
				gap := (next.X - (prev.X + prev.W)) * page.Width
				height := max(prev.H, next.H) * page.Height
				split = gap > gapInLetters*height
			}
			if !split {
				continue
			}
			group := words[start:i]
			start = i
			from, to := group[0].At, group[0].At+group[0].Len
			x0, y0, x1, y1 := group[0].X, group[0].Y, group[0].X+group[0].W, group[0].Y+group[0].H
			for _, w := range group[1:] {
				from, to = min(from, w.At), max(to, w.At+w.Len)
				x0, y0 = min(x0, w.X), min(y0, w.Y)
				x1, y1 = max(x1, w.X+w.W), max(y1, w.Y+w.H)
			}
			if from < 0 || to > len(units) || from >= to {
				continue
			}
			if len(group) == len(words) {
				from, to = 0, len(units)
			}
			text := latin(strings.TrimSpace(string(utf16.Decode(units[from:to]))))
			if text == "" {
				continue
			}
			out = append(out, Word{Text: text, X: x0, Y: y0, W: x1 - x0, H: y1 - y0})
		}
	}
	return out
}

var lookalikes = strings.NewReplacer(
	"А", "A", "В", "B", "С", "C", "Е", "E", "Н", "H", "І", "I", "Ј", "J", "К", "K", "М", "M",
	"О", "O", "Р", "P", "Ѕ", "S", "Т", "T", "Х", "X", "Υ", "Y", "Ζ", "Z",
	"а", "a", "с", "c", "е", "e", "і", "i", "ј", "j", "о", "o", "р", "p", "ѕ", "s", "х", "x", "у", "y",
)

func latin(text string) string {
	latinLetters, others := 0, 0
	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Latin):
			latinLetters++
		case unicode.IsLetter(r):
			others++
		}
	}
	if latinLetters == 0 || others > latinLetters {
		return text
	}
	return lookalikes.Replace(text)
}

func SameLabel(a, b string) bool {
	la, lb := labelOf(a), labelOf(b)
	return la == lb || strings.ReplaceAll(la, " ", "") == strings.ReplaceAll(lb, " ", "")
}

func labelOf(s string) string {
	s = strings.ReplaceAll(s, "\u2026", "...")
	s = strings.ToLower(strings.TrimSpace(latin(s)))
	s = strings.TrimRight(s, ".:")
	return strings.Join(strings.Fields(s), " ")
}
