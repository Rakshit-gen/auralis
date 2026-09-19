package internal

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	for addr, want := range map[string]string{
		"10.0.0.1:5555":    "10.0.0.1",
		"[2001:db8::1]:80": "2001:db8::1",
		"bare":             "bare",
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = addr
		if got := clientIP(r); got != want {
			t.Errorf("%q: got %q want %q", addr, got, want)
		}
	}
}
