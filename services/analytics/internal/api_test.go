package internal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/auralis/platform/authn"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testIdentitySecret = "test-identity-secret"

func newAPIServer(t *testing.T, pool *pgxpool.Pool) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	r.Use(authn.ServiceMiddleware(testIdentitySecret))
	NewAPI(pool).Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func getAs(t *testing.T, url string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := authn.Identity{UserID: "aaaaaaaa-0000-0000-0000-000000000009", Roles: []string{authn.RoleUser}}
	for k, v := range authn.IdentityHeaders(testIdentitySecret, id, "req-test") {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// TestPerformanceLookupErrorIsNotMaskedAs404 guards the fix for the show/episode
// performance handlers that mapped every QueryRow error to 404. A malformed id
// makes Postgres reject the query (22P02): a 500, not a "not found".
func TestPerformanceLookupErrorIsNotMaskedAs404(t *testing.T) {
	_, pool := newAgg(t)
	srv := newAPIServer(t, pool)

	if got := getAs(t, srv.URL+"/analytics/shows/not-a-uuid"); got != http.StatusInternalServerError {
		t.Fatalf("malformed show id: expected 500, got %d", got)
	}
	if got := getAs(t, srv.URL+"/analytics/episodes/not-a-uuid"); got != http.StatusInternalServerError {
		t.Fatalf("malformed episode id: expected 500, got %d", got)
	}

	// A well-formed but absent id is still a genuine 404.
	absent := "99999999-9999-9999-9999-999999999999"
	if got := getAs(t, srv.URL+"/analytics/shows/"+absent); got != http.StatusNotFound {
		t.Fatalf("absent show id: expected 404, got %d", got)
	}
	if got := getAs(t, srv.URL+"/analytics/episodes/"+absent); got != http.StatusNotFound {
		t.Fatalf("absent episode id: expected 404, got %d", got)
	}
}
