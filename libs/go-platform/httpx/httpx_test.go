package httpx

import (
	"net/http"
	"testing"
)

func TestClientIPHandlesIPv6RemoteAddr(t *testing.T) {
	for addr, want := range map[string]string{
		"192.0.2.1:5555":      "192.0.2.1",
		"[2001:db8::1]:54321": "2001:db8::1",
	} {
		r := &http.Request{RemoteAddr: addr, Header: http.Header{}}
		if got := clientIP(r); got != want {
			t.Errorf("clientIP(%q) = %q, want %q", addr, got, want)
		}
	}

	// X-Forwarded-For still wins and is trimmed to the leftmost hop.
	r := &http.Request{RemoteAddr: "[2001:db8::1]:1", Header: http.Header{}}
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.7" {
		t.Errorf("XFF: got %q, want 203.0.113.7", got)
	}
}
