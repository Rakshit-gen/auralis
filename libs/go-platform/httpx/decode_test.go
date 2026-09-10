package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/auralis/platform/errcodes"
)

func TestDecodeOversizeBodyReportsTooLarge(t *testing.T) {
	body := `{"x":"` + strings.Repeat("a", MaxBodyBytes+1024) + `"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()

	err := Decode(w, r, &struct {
		X string `json:"x"`
	}{})
	e, ok := err.(*errcodes.Error)
	if !ok || e.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 payload-too-large, got %v", err)
	}
}

func TestDecodeEmptyBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	err := Decode(httptest.NewRecorder(), r, &struct{}{})
	e, ok := err.(*errcodes.Error)
	if !ok || e.Status != http.StatusBadRequest {
		t.Fatalf("want 400 for empty body, got %v", err)
	}
}
