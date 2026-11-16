package internal

import "testing"

func TestCheckUpload(t *testing.T) {
	a := &App{MaxUploadMB: 1}
	cases := []struct {
		size           int64
		stored, wanted string
		ok             bool
	}{
		{4096, "audio/mpeg", "audio/mpeg", true},
		{4096, "Audio/MPEG; charset=binary", "audio/mpeg", true},
		{100, "audio/mpeg", "audio/mpeg", false},
		{2 << 20, "audio/mpeg", "audio/mpeg", false},
		{4096, "text/html", "audio/mpeg", false},
	}
	for _, c := range cases {
		if got := a.checkUpload(c.size, c.stored, c.wanted) == ""; got != c.ok {
			t.Errorf("checkUpload(%d, %q, %q) ok=%v, want %v", c.size, c.stored, c.wanted, got, c.ok)
		}
	}
}
