package internal

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestStatusChecksAreShared(t *testing.T) {
	var hits atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer backend.Close()

	h := SystemStatusHandler(map[string]string{"a": backend.URL, "b": backend.URL})
	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	}
	// Two backends, /health and /ready each, checked once.
	if got := hits.Load(); got != 4 {
		t.Fatalf("backends were called %d times for 20 requests, want 4", got)
	}
}
