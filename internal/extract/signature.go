package extract

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"golang.org/x/net/html"
)

// SigOptions tunes signature sensitivity.
//
// This is a knife-edge. Too sensitive and every trivial DOM change invalidates
// the cache, so synthesis runs on every visit and the runtime is never faster
// than an LLM. Too loose and a redesigned page keeps replaying selectors that
// no longer match, returning wrong data confidently.
type SigOptions struct {
	// MaxDepth bounds how deep structure is recorded. Deep tails are mostly
	// presentational and change more often than they mean anything.
	MaxDepth int // defaults to 12

	// CollapseRepeats folds runs of same-shaped siblings into one entry, so a
	// search returning 10 results signs the same as one returning 25.
	// Without this the cache almost never hits on real sites.
	CollapseRepeats bool // defaults to true via NewSigOptions
}

// NewSigOptions returns the defaults.
func NewSigOptions() SigOptions {
	return SigOptions{MaxDepth: 12, CollapseRepeats: true}
}

func (o SigOptions) withDefaults() SigOptions {
	if o.MaxDepth == 0 {
		o.MaxDepth = 12
	}
	return o
}

// Signature hashes a document's structure, ignoring all text content.
//
// Two pages of the same site with different data produce the same signature.
// A redesign produces a different one. That difference is what makes a learned
// extraction program safe to replay.
func Signature(root *html.Node, opts SigOptions) string {
	opts = opts.withDefaults()

	var b strings.Builder
	writeSkeleton(&b, root, 0, opts)

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:16]
}

// SignatureOf parses HTML and signs it.
func SignatureOf(htmlSrc string, opts SigOptions) (string, error) {
	root, err := Parse(htmlSrc)
	if err != nil {
		return "", err
	}
	return Signature(root, opts), nil
}

// writeSkeleton emits a canonical, text-free rendering of the tree.
func writeSkeleton(b *strings.Builder, n *html.Node, depth int, opts SigOptions) {
	if depth > opts.MaxDepth {
		return
	}

	if n.Type == html.ElementNode {
		if skipTags[n.Data] {
			return
		}
		b.WriteString(structureKey(n))
		b.WriteByte('(')
	}

	writeChildren(b, n, depth, opts)

	if n.Type == html.ElementNode {
		b.WriteByte(')')
	}
}

func writeChildren(b *strings.Builder, n *html.Node, depth int, opts SigOptions) {
	var prevKey string
	var repeats int

	flush := func() {
		// A collapsed run is marked so that "many" stays distinguishable from
		// "one", without the exact count leaking into the hash.
		if repeats > 1 {
			b.WriteString("*")
		}
		prevKey, repeats = "", 0
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || skipTags[c.Data] {
			continue
		}

		if opts.CollapseRepeats {
			key := structureKey(c)
			if key == prevKey {
				repeats++
				continue // structure already recorded by the first of the run
			}
			flush()
			prevKey, repeats = key, 1
		}

		writeSkeleton(b, c, depth+1, opts)
	}

	if opts.CollapseRepeats {
		flush()
	}
}

// structureKey is an element's identity for signing: tag plus stable classes,
// with build-hashed names normalized away so a rebuild does not read as a
// redesign.
func structureKey(n *html.Node) string {
	var b strings.Builder
	b.WriteString(n.Data)
	for _, c := range classes(n) {
		b.WriteByte('.')
		b.WriteString(c)
	}
	return b.String()
}
