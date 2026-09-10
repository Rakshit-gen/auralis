package config

import "testing"

func TestOptionalBoolRejectsGarbageLikeSiblings(t *testing.T) {
	t.Setenv("FLAG_GOOD", "yes")
	t.Setenv("FLAG_TYPO", "treu")

	r := New()
	if got := r.OptionalBool("FLAG_GOOD", false); got != true {
		t.Fatalf("FLAG_GOOD: want true, got %v", got)
	}
	if got := r.OptionalBool("FLAG_UNSET", true); got != true {
		t.Fatalf("FLAG_UNSET: want fallback true, got %v", got)
	}
	// A garbage value must fail startup, not silently use the fallback.
	_ = r.OptionalBool("FLAG_TYPO", true)
	if r.Err() == nil {
		t.Fatal("garbage boolean value did not produce a startup error")
	}
}
