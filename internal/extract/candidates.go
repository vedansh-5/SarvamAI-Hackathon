package extract

import (
	"math"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// MinRecords is the smallest run of matching siblings treated as a data
// region. Two could be coincidence; three is a pattern.
const MinRecords = 3

// Candidate is a repeating region of the page that may hold the data the user
// asked for. Choosing between candidates is the model's job; finding them is
// not.
type Candidate struct {
	// Selector matches the first element of every record in the region.
	Selector string

	// Span is how many consecutive sibling elements make up one record. One
	// for the ordinary case; more when a record is split across siblings, as
	// in a table that puts a row's details in the row beneath it.
	Span int

	// RecordCount is how many records were found.
	RecordCount int

	// Skeleton is a compact structural outline of the first record.
	Skeleton string

	// Samples are the plain-text contents of the first few records.
	Samples []string

	// Score ranks candidates; higher is more likely to be the main content.
	Score float64
}

// Options tunes candidate discovery.
type Options struct {
	MinRecords    int // defaults to MinRecords
	MaxCandidate  int // how many candidates to return; defaults to 8
	MaxSamples    int // sample records per candidate; defaults to 3
	SkeletonDepth int // defaults to 4

	// MaxSpan bounds how many consecutive siblings may form one record.
	// Defaults to MaxRootSpan.
	MaxSpan int
}

func (o Options) withDefaults() Options {
	if o.MinRecords == 0 {
		o.MinRecords = MinRecords
	}
	if o.MaxCandidate == 0 {
		o.MaxCandidate = 8
	}
	if o.MaxSamples == 0 {
		o.MaxSamples = 3
	}
	if o.SkeletonDepth == 0 {
		o.SkeletonDepth = 4
	}
	if o.MaxSpan == 0 {
		o.MaxSpan = MaxRootSpan
	}
	return o
}

// Candidates finds repeating data regions in a parsed document.
func Candidates(root *html.Node, opts Options) []Candidate {
	opts = opts.withDefaults()

	var found []Candidate
	var walk func(*html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[n.Data] {
			return
		}
		if n.Type == html.ElementNode || n.Type == html.DocumentNode {
			found = append(found, groupsIn(n, opts)...)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	sort.SliceStable(found, func(i, j int) bool {
		return found[i].Score > found[j].Score
	})

	found = dedupe(found)
	if len(found) > opts.MaxCandidate {
		found = found[:opts.MaxCandidate]
	}
	return found
}

// groupsIn finds repeating record shapes among one parent's children.
//
// Two shapes are looked for. Single-element records are grouped by
// fingerprint, which does not require them to be adjacent — a list whose items
// are interrupted by an ad slot is still a list. Multi-element records are
// found by looking for a repeating period among consecutive siblings, which
// does require adjacency, because that is what makes the grouping meaningful.
func groupsIn(parent *html.Node, opts Options) []Candidate {
	children := elementChildren(parent)
	if len(children) < opts.MinRecords {
		return nil
	}

	out := singleGroups(parent, children, opts)
	for span := 2; span <= opts.MaxSpan; span++ {
		if len(children) < span*opts.MinRecords {
			break
		}
		out = append(out, windowGroups(parent, children, span, opts)...)
	}
	return out
}

// elementChildren returns the children worth considering as record material.
func elementChildren(parent *html.Node) []*html.Node {
	var out []*html.Node
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && !skipTags[c.Data] && fingerprint(c) != "" {
			out = append(out, c)
		}
	}
	return out
}

// singleGroups finds records made of one element each.
func singleGroups(parent *html.Node, children []*html.Node, opts Options) []Candidate {
	groups := map[string][]*html.Node{}
	var order []string

	for _, c := range children {
		fp := fingerprint(c)
		if _, seen := groups[fp]; !seen {
			order = append(order, fp)
		}
		groups[fp] = append(groups[fp], c)
	}

	var out []Candidate
	for _, fp := range order {
		records := groups[fp]
		if len(records) < opts.MinRecords {
			continue
		}
		windows := make([][]*html.Node, len(records))
		for i, r := range records {
			windows[i] = []*html.Node{r}
		}
		if c, ok := buildCandidate(parent, windows, 1, opts); ok {
			out = append(out, c)
		}
	}
	return out
}

// windowGroups finds records made of span consecutive siblings.
//
// Every starting offset is tried, because the repeating unit does not
// necessarily begin at the first child — a table often opens with a header row
// that is not part of any record.
func windowGroups(parent *html.Node, children []*html.Node, span int, opts Options) []Candidate {
	var out []Candidate

	for offset := 0; offset < span; offset++ {
		var keys []string
		var starts []int
		for i := offset; i+span <= len(children); i += span {
			keys = append(keys, windowKey(children[i:i+span]))
			starts = append(starts, i)
		}

		// Maximal runs of identical windows. A run, not merely a count: the
		// records of one region are adjacent, and two separated groups that
		// happen to share a shape are two regions.
		for s := 0; s < len(keys); {
			e := s
			for e+1 < len(keys) && keys[e+1] == keys[s] {
				e++
			}
			if n := e - s + 1; n >= opts.MinRecords {
				windows := make([][]*html.Node, n)
				for i := 0; i < n; i++ {
					at := starts[s+i]
					windows[i] = children[at : at+span]
				}
				if c, ok := buildCandidate(parent, windows, span, opts); ok {
					out = append(out, c)
				}
			}
			s = e + 1
		}
	}
	return out
}

// windowKey identifies a window's shape.
func windowKey(window []*html.Node) string {
	var b strings.Builder
	for i, n := range window {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(fingerprint(n))
	}
	return b.String()
}

// buildCandidate turns a set of record windows into a Candidate, rejecting
// the ones that cannot be replayed or are not data.
func buildCandidate(parent *html.Node, windows [][]*html.Node, span int, opts Options) (Candidate, bool) {
	first := windows[0][0]

	// A record has to be findable again by selector. If the first element's
	// selector matches more elements under this parent than there are records,
	// replaying it would produce record boundaries in the wrong places — and
	// with a span, overlapping ones. This is what rules out a bare "tr" as the
	// start of a record while allowing "tr.athing".
	if span > 1 && countMatching(parent, first) != len(windows) {
		return Candidate{}, false
	}

	// A record that opens with an empty element has the wrong boundary. Every
	// rotation of the period is tried, so when a region really does contain a
	// spacer, the rotation that starts on content is found instead.
	if span > 1 && text(first) == "" {
		return Candidate{}, false
	}

	var samples []string
	var totalText int
	for i, w := range windows {
		t := windowText(w)
		totalText += len(t)
		if i < opts.MaxSamples && t != "" {
			if len(t) > 200 {
				t = t[:200] + "…"
			}
			samples = append(samples, t)
		}
	}

	// A region of empty siblings — spacers, layout scaffolding — is not data
	// no matter how many of them there are.
	if totalText == 0 {
		return Candidate{}, false
	}

	var sk strings.Builder
	for _, n := range windows[0] {
		sk.WriteString(skeleton(n, opts.SkeletonDepth))
	}

	avgText := float64(totalText) / float64(len(windows))

	return Candidate{
		Selector:    recordSelector(parent, first),
		Span:        span,
		RecordCount: len(windows),
		Skeleton:    sk.String(),
		Samples:     samples,
		Score:       float64(len(windows)) * math.Log1p(avgText),
	}, true
}

// countMatching counts the parent's children sharing n's selector shape.
func countMatching(parent *html.Node, n *html.Node) int {
	want := selectorFor(n)
	count := 0
	for _, c := range elementChildren(parent) {
		if selectorFor(c) == want {
			count++
		}
	}
	return count
}

// windowText is the visible text of a whole record.
func windowText(window []*html.Node) string {
	parts := make([]string, 0, len(window))
	for _, n := range window {
		if t := text(n); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

// recordSelector describes the records relative to their container.
func recordSelector(parent, record *html.Node) string {
	child := selectorFor(record)
	if parent.Type != html.ElementNode {
		return child
	}
	p := cssPath(parent)
	if p == "" {
		return child
	}
	return p + " > " + child
}

// dedupe drops candidates whose selector is a prefix of a higher-scoring one.
// Nested regions otherwise report the same data several times over.
func dedupe(in []Candidate) []Candidate {
	var out []Candidate
	for _, c := range in {
		redundant := false
		for _, kept := range out {
			if strings.HasPrefix(c.Selector, kept.Selector) || strings.HasPrefix(kept.Selector, c.Selector) {
				redundant = true
				break
			}
		}
		if !redundant {
			out = append(out, c)
		}
	}
	return out
}

// FindCandidates parses HTML and returns its data regions.
func FindCandidates(htmlSrc string, opts Options) ([]Candidate, error) {
	root, err := Parse(htmlSrc)
	if err != nil {
		return nil, err
	}
	return Candidates(root, opts), nil
}
