//go:build integration

package synth

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/extract"
)

// Run with: go test -tags integration ./internal/synth/
//
// Makes a real API call. Needs ANTHROPIC_API_KEY, or an active profile from
// `ant auth login`.
func TestSynthesizeLive(t *testing.T) {
	html, err := os.ReadFile("../extract/testdata/products.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	candidates, err := extract.FindCandidates(string(html), extract.Options{})
	if err != nil {
		t.Fatalf("FindCandidates: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("no candidates in the fixture")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	got, err := NewAnthropic().Synthesize(ctx, Request{
		Intent:     "the product name, link and price for each item",
		URL:        "https://shop.example/search",
		Candidates: candidates,
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Validate already ran inside Synthesize; these check it picked the right
	// region and understood the task, not merely that it returned valid JSON.
	if !strings.Contains(got.Root, "card") {
		t.Errorf("root = %q, expected it to target the product cards", got.Root)
	}
	if len(got.Fields) < 2 {
		t.Errorf("only %d fields: %+v", len(got.Fields), got.Fields)
	}

	var sawHref bool
	for _, f := range got.Fields {
		if f.Attr == "href" {
			sawHref = true
		}
	}
	if !sawHref {
		t.Errorf("no field reads href, though a link was requested: %+v", got.Fields)
	}

	t.Logf("root=%q fields=%v expect=%+v", got.Root, got.FieldNames(), got.Expect)
}
