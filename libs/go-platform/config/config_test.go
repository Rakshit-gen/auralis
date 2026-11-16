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

func TestRequireSecret(t *testing.T) {
	cases := []struct {
		value, allowDev string
		ok              bool
	}{
		{"short", "", false},
		{"dev-jwt-secret-change-me-0123456789", "", false},
		{"dev-jwt-secret-change-me-0123456789", "true", true},
		{"9f8e7d6c5b4a39281706f5e4d3c2b1a0aa", "", true},
	}
	for _, c := range cases {
		t.Setenv("TEST_SECRET", c.value)
		t.Setenv("ALLOW_DEV_SECRETS", c.allowDev)
		r := New()
		r.RequireSecret("TEST_SECRET")
		if got := r.Err() == nil; got != c.ok {
			t.Errorf("RequireSecret(%q) with ALLOW_DEV_SECRETS=%q: ok=%v, want %v (%v)", c.value, c.allowDev, got, c.ok, r.Err())
		}
	}
}
