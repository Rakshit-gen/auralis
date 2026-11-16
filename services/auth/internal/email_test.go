package internal

import "testing"

func TestNormalizeEmailRejectsDisplayNameForms(t *testing.T) {
	for in, want := range map[string]string{
		" Ada@Example.com ":         "ada@example.com",
		"Ada <ada@example.com>":     "",
		"<ada@example.com>":         "",
		"not an email":              "",
		"\"Ada\" <ada@example.com>": "",
	} {
		if got := normalizeEmail(in); got != want {
			t.Errorf("normalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
