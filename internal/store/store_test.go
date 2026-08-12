package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/extract"
)

func testStore(t *testing.T) *Store {
	t.Helper()

	// A file, not :memory:, so persistence across Open is exercised by
	// whichever test wants it and the WAL pragmas are real.
	s, err := Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// A fixed, advancing clock: ordering by last_success_at is meaningless if
	// two writes in the same second are indistinguishable.
	now := time.Unix(1_700_000_000, 0)
	s.Now = func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	return s
}

func prog(root string) extract.Program {
	return extract.Program{
		Root:   root,
		Fields: []extract.Field{{Name: "name", Selector: "h3", Attr: "text"}},
		Expect: extract.Expectation{MinRows: 1, Required: []string{"name"}},
	}
}

func testKey(t *testing.T) Key {
	t.Helper()
	k, err := NewKey("https://shop.example/search?q=widget", "abc123", "product names")
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	return k
}

func TestPutThenLookup(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	key := testKey(t)

	id, err := s.Put(ctx, key, prog("article.card"), "product names", "gemma")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Lookup(ctx, key)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].ID != id {
		t.Errorf("id = %d, want %d", got[0].ID, id)
	}
	if got[0].Program.Root != "article.card" {
		t.Errorf("root = %q", got[0].Program.Root)
	}
	if got[0].Intent != "product names" || got[0].Model != "gemma" {
		t.Errorf("provenance lost: intent=%q model=%q", got[0].Intent, got[0].Model)
	}
}

func TestLookupMissIsNotAnError(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	got, err := s.Lookup(ctx, testKey(t))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries from an empty store", len(got))
	}
}

// The query string is what changes between one search and the next. Keying on
// it would mean the cache never hits.
func TestSameOriginDifferentPathHitsTheSameKey(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	k1, _ := NewKey("https://shop.example/search?q=widget", "abc123", "product names")
	k2, _ := NewKey("https://shop.example/search?q=gadget&page=3", "abc123", "product names")

	if _, err := s.Put(ctx, k1, prog("article.card"), "product names", "gemma"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Lookup(ctx, k2)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("a second search on the same site missed the cache")
	}
}

func TestDifferentOriginDoesNotShareAProgram(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	k1, _ := NewKey("https://shop.example/search", "abc123", "product names")
	k2, _ := NewKey("https://other.example/search", "abc123", "product names")

	if _, err := s.Put(ctx, k1, prog("article.card"), "product names", "gemma"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, _ := s.Lookup(ctx, k2)
	if len(got) != 0 {
		t.Errorf("a program leaked across origins on a shared DOM skeleton")
	}
}

func TestDifferentSignatureMisses(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	k1, _ := NewKey("https://shop.example/search", "abc123", "product names")
	k2, _ := NewKey("https://shop.example/search", "def456", "product names")

	if _, err := s.Put(ctx, k1, prog("article.card"), "product names", "gemma"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, _ := s.Lookup(ctx, k2)
	if len(got) != 0 {
		t.Errorf("a redesigned page replayed the old program")
	}
}

func TestDifferentIntentMisses(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	k1, _ := NewKey("https://shop.example/search", "abc123", "product names")
	k2, _ := NewKey("https://shop.example/search", "abc123", "shipping estimates")

	if _, err := s.Put(ctx, k1, prog("article.card"), "product names", "gemma"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, _ := s.Lookup(ctx, k2)
	if len(got) != 0 {
		t.Errorf("one page shape answered a different question from cache")
	}
}

func TestIntentNormalization(t *testing.T) {
	same := [][2]string{
		{"Product names and prices", "product names and prices"},
		{"product names and prices.", "product names and prices"},
		{"product  names\tand\nprices", "product names and prices"},
		{"product names, and prices!", "product names and prices"},
	}
	for _, pair := range same {
		if IntentHash(pair[0]) != IntentHash(pair[1]) {
			t.Errorf("%q and %q hashed differently", pair[0], pair[1])
		}
	}

	// Shallow normalization must not merge genuinely different questions.
	if IntentHash("product names") == IntentHash("product prices") {
		t.Error("two different intents collided")
	}
}

func TestReSynthesisKeepsTheOlderProgram(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	key := testKey(t)

	first, err := s.Put(ctx, key, prog("article.card"), "product names", "gemma")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	second, err := s.Put(ctx, key, prog("li.product"), "product names", "gemma")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if first == second {
		t.Fatal("the second program replaced the first")
	}

	got, _ := s.Lookup(ctx, key)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want both programs kept: %+v", len(got), got)
	}
}

// A model handed failure reasons sometimes returns exactly what it just wrote.
// Re-inserting it would reset the failure count that proves it does not work.
func TestPutIsIdempotentForAnIdenticalProgram(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	key := testKey(t)

	first, _ := s.Put(ctx, key, prog("article.card"), "product names", "gemma")
	if err := s.RecordRun(ctx, first, false); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	again, err := s.Put(ctx, key, prog("article.card"), "product names", "gemma")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if again != first {
		t.Errorf("id = %d, want the existing %d", again, first)
	}

	got, _ := s.Lookup(ctx, key)
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].ConsecutiveFailures != 1 {
		t.Errorf("failure count was reset by the re-insert: %+v", got[0])
	}
}

// Ordering is the whole point of keeping several: the one most likely to work
// has to be tried first, or the extra entries only cost time.
func TestLookupOrdersWorkingProgramsFirst(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	key := testKey(t)

	broken, _ := s.Put(ctx, key, prog("article.card"), "product names", "gemma")
	working, _ := s.Put(ctx, key, prog("li.product"), "product names", "gemma")

	if err := s.RecordRun(ctx, broken, false); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	if err := s.RecordRun(ctx, working, true); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	got, _ := s.Lookup(ctx, key)
	if len(got) != 2 {
		t.Fatalf("got %d entries", len(got))
	}
	if got[0].ID != working {
		t.Errorf("failing program ordered first: %+v", got)
	}
}

// A program that worked for months and broke once should not fall behind one
// that has never worked.
func TestConsecutiveFailuresResetOnSuccess(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	key := testKey(t)

	id, _ := s.Put(ctx, key, prog("article.card"), "product names", "gemma")
	for i := 0; i < 3; i++ {
		if err := s.RecordRun(ctx, id, false); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}
	if err := s.RecordRun(ctx, id, true); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	got, _ := s.Lookup(ctx, key)
	e := got[0]
	if e.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0 after a success", e.ConsecutiveFailures)
	}
	if e.Failures != 3 || e.Runs != 4 {
		t.Errorf("lifetime totals wrong: runs=%d failures=%d", e.Runs, e.Failures)
	}
	if e.LastSuccessAt.IsZero() {
		t.Error("LastSuccessAt was not set")
	}
}

func TestPruneKeepsTheBest(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	key := testKey(t)

	good, _ := s.Put(ctx, key, prog("article.card"), "product names", "gemma")
	if err := s.RecordRun(ctx, good, true); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	for _, root := range []string{"li.a", "li.b", "li.c", "li.d", "li.e"} {
		id, err := s.Put(ctx, key, prog(root), "product names", "gemma")
		if err != nil {
			t.Fatalf("Put %s: %v", root, err)
		}
		if err := s.RecordRun(ctx, id, false); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}

	got, _ := s.Lookup(ctx, key)
	if len(got) > MaxProgramsPerKey {
		t.Fatalf("got %d entries, want at most %d", len(got), MaxProgramsPerKey)
	}
	if got[0].ID != good {
		t.Errorf("the one program known to work was not kept first: %+v", got)
	}
}

func TestInvalidateSignatureClearsEveryIntent(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	k1, _ := NewKey("https://shop.example/search", "abc123", "product names")
	k2, _ := NewKey("https://shop.example/search", "abc123", "shipping estimates")
	other, _ := NewKey("https://shop.example/search", "zzz999", "product names")

	for _, k := range []Key{k1, k2, other} {
		if _, err := s.Put(ctx, k, prog("article.card"), "x", "gemma"); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	n, err := s.InvalidateSignature(ctx, k1.Origin, k1.Signature)
	if err != nil {
		t.Fatalf("InvalidateSignature: %v", err)
	}
	if n != 2 {
		t.Errorf("removed %d programs, want 2", n)
	}

	if got, _ := s.Lookup(ctx, k2); len(got) != 0 {
		t.Error("a second intent on the discredited signature survived")
	}
	if got, _ := s.Lookup(ctx, other); len(got) != 1 {
		t.Error("an unrelated signature was invalidated too")
	}
}

// program.go promises Validate runs on programs loaded from cache. A program
// outlives the schema that produced it, and replaying one the runner cannot
// execute would surface as a page failure rather than as what it is.
func TestUnusableStoredProgramsAreDropped(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	key := testKey(t)

	good, _ := s.Put(ctx, key, prog("article.card"), "product names", "gemma")

	// Written directly: Put would refuse both of these, which is the point.
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO programs (origin, signature, intent_hash, intent, program, model, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?)`,
		key.Origin, key.Signature, key.IntentHash, "x", `{"root":"","fields":[]}`, "old", 1,
		key.Origin, key.Signature, key.IntentHash, "x", `not json at all`, "old", 1)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := s.Lookup(ctx, key)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 1 || got[0].ID != good {
		t.Fatalf("got %+v, want only the usable program", got)
	}

	// And they are gone, not merely skipped — otherwise every lookup pays for
	// them forever.
	if st, _ := s.Stats(ctx); st.Programs != 1 {
		t.Errorf("Programs = %d, want the unusable rows deleted", st.Programs)
	}
}

func TestPutRejectsAnInvalidProgram(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	bad := prog("article.card")
	bad.Expect.Required = []string{"nonexistent"}

	if _, err := s.Put(ctx, testKey(t), bad, "product names", "gemma"); err == nil {
		t.Fatal("an invalid program was cached")
	}
}

func TestSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "vans.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	key := testKey(t)
	if _, err := s.Put(ctx, key, prog("article.card"), "product names", "gemma"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	got, err := reopened.Lookup(ctx, key)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries after reopen — the cache does not survive a restart", len(got))
	}
}

func TestNewKeyRejectsUnusableInput(t *testing.T) {
	cases := []struct{ url, sig, intent string }{
		{"", "abc", "names"},
		{"not-a-url", "abc", "names"},
		{"https://shop.example", "", "names"},
		{"https://shop.example", "abc", "   "},
	}
	for _, c := range cases {
		if _, err := NewKey(c.url, c.sig, c.intent); err == nil {
			t.Errorf("NewKey(%q, %q, %q) succeeded", c.url, c.sig, c.intent)
		}
	}
}

func TestRecordRunOnAMissingProgram(t *testing.T) {
	if err := testStore(t).RecordRun(context.Background(), 999, true); err == nil {
		t.Error("recording a run against a deleted program should not silently pass")
	}
}
