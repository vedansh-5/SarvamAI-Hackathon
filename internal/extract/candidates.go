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
	// Selector matches every record in the region.
	Selector string

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

// groupsIn finds sets of same-shaped children under one parent.
func groupsIn(parent *html.Node, opts Options) []Candidate {
	groups := map[string][]*html.Node{}
	var order []string

	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || skipTags[c.Data] {
			continue
		}
		fp := fingerprint(c)
		if fp == "" {
			continue
		}
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

		var samples []string
		var totalText int
		for i, r := range records {
			t := text(r)
			totalText += len(t)
			if i < opts.MaxSamples && t != "" {
				if len(t) > 200 {
					t = t[:200] + "…"
				}
				samples = append(samples, t)
			}
		}

		// A region of empty siblings — spacers, layout scaffolding — is not
		// data no matter how many of them there are.
		if totalText == 0 {
			continue
		}

		avgText := float64(totalText) / float64(len(records))

		out = append(out, Candidate{
			Selector:    recordSelector(parent, records[0]),
			RecordCount: len(records),
			Skeleton:    skeleton(records[0], opts.SkeletonDepth),
			Samples:     samples,
			Score:       float64(len(records)) * math.Log1p(avgText),
		})
	}
	return out
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
