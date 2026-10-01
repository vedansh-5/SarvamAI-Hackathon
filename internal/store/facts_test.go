package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWhatIsRememberedCanBeReadBackAndForgotten(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for _, fact := range []string{"my home city is Delhi", "mom is Sunita Sharma on WhatsApp", "my home city is Delhi", "  "} {
		if err := s.Remember(ctx, fact); err != nil {
			t.Fatal(err)
		}
	}
	facts, _ := s.Facts(ctx)
	if len(facts) != 2 {
		t.Fatalf("facts = %v, want two; the same thing said twice is one thing", facts)
	}

	gone, err := s.Forget(ctx, "MOM")
	if err != nil || gone != 1 {
		t.Fatalf("forgot %d, err %v", gone, err)
	}
	facts, _ = s.Facts(ctx)
	if len(facts) != 1 || facts[0] != "my home city is Delhi" {
		t.Errorf("facts = %v", facts)
	}
}
