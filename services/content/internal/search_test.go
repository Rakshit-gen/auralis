package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchRejectsBlankAndOneCharQueries(t *testing.T) {
	a := &App{}
	for _, q := range []string{"", "a", "%20%20", "%20a%20", "%C3%A9"} {
		w := httptest.NewRecorder()
		a.search(w, httptest.NewRequest(http.MethodGet, "/v1/search?q="+q, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("q=%q: status %d, want 400", q, w.Code)
		}
	}
}
