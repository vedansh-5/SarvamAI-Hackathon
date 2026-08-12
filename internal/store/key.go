// Package store persists synthesized programs so they are written once and
// replayed forever.
//
// This is where the runtime's central claim becomes a real thing rather than
// an intention. A cold run calls the model; every warm run on the same page
// shape must call it zero times, and that only holds if a lookup keyed on
// something stable actually finds what was written earlier.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// Key identifies a cached program.
//
// All three parts are necessary. The signature alone is not enough because one
// page shape answers many different questions, and the intent alone is not
// enough because the same question against a redesigned page needs a different
// program. Origin scopes both: two unrelated sites can share a DOM skeleton,
// and a program written for one has no business running on the other.
type Key struct {
	// Origin is scheme://host, without path or query.
	Origin string

	// Signature is the page's structural hash (see extract.SignatureOf).
	Signature string

	// IntentHash is the normalized user intent, hashed.
	IntentHash string
}

// NewKey builds a Key from a page URL, its signature, and the user's intent.
func NewKey(pageURL, signature, intent string) (Key, error) {
	if strings.TrimSpace(signature) == "" {
		return Key{}, fmt.Errorf("store: empty page signature")
	}
	if strings.TrimSpace(intent) == "" {
		return Key{}, fmt.Errorf("store: empty intent")
	}

	origin, err := OriginOf(pageURL)
	if err != nil {
		return Key{}, err
	}

	return Key{
		Origin:     origin,
		Signature:  signature,
		IntentHash: IntentHash(intent),
	}, nil
}

// OriginOf reduces a URL to scheme://host.
//
// The path is deliberately excluded. A search results page differs from every
// other search results page only in its query string, and keying on the full
// URL would mean a cache that never hits — every new search would be a cold
// run, which is exactly the outcome this package exists to prevent. The page
// signature is what distinguishes one page shape from another; the URL only
// says which site it is.
func OriginOf(pageURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil {
		return "", fmt.Errorf("store: parse page URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("store: page URL %q has no scheme or host", pageURL)
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}

// IntentHash normalizes an intent and hashes it.
//
// Normalization is deliberately shallow: case folded, punctuation dropped,
// whitespace collapsed. Nothing here tries to understand the sentence.
//
// The consequence is that paraphrases miss. "product names and prices" and
// "the name and price of each product" ask the same question and will be
// synthesized separately. That is accepted for now because the alternatives
// are worse: semantic matching needs an embedding model, which is a network
// call on the warm path, and a warm path that calls a model is the thing this
// whole design is built to avoid. A miss costs one synthesis and is then
// cached; a false hit runs the wrong program and reports it as a success.
func IntentHash(intent string) string {
	sum := sha256.Sum256([]byte(NormalizeIntent(intent)))
	return hex.EncodeToString(sum[:])[:16]
}

// NormalizeIntent folds an intent to its comparison form.
func NormalizeIntent(intent string) string {
	var b strings.Builder
	b.Grow(len(intent))

	for _, r := range strings.ToLower(intent) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// String renders the key for logs.
func (k Key) String() string {
	return fmt.Sprintf("%s#%s/%s", k.Origin, k.Signature, k.IntentHash)
}
