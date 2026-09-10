package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/auralis/content/migrations"
	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/migrate"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testIdentitySecret = "content-test-identity"

func testDBURL() string {
	if v := os.Getenv("CONTENT_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://localhost:5432/content_test?sslmode=disable"
}

func setup(t *testing.T) (*httptest.Server, *App, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDBURL())
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		t.Skipf("content_test database not reachable (%v)", err)
	}
	if err := migrate.Run(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, err = pool.Exec(ctx, `TRUNCATE shows, seasons, episodes, creators, review_events,
		media_uploads, outbox_events, processed_events RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	seedRef(t, pool)
	t.Cleanup(pool.Close)

	app := NewApp(NewStore(pool), nil, "svc-token")
	r := chi.NewRouter()
	r.Use(authn.ServiceMiddleware(testIdentitySecret))
	app.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, app, pool
}

func seedRef(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO languages (code, name) VALUES ('en','English'),('hi','Hindi')
		 ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO genres (slug, name) VALUES ('mystery','Mystery'),('sci-fi','Science Fiction')
		 ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
}

func hdr(userID string, roles ...string) map[string]string {
	return authn.IdentityHeaders(testIdentitySecret, authn.Identity{UserID: userID, Roles: roles}, "req")
}

func do(t *testing.T, method, url string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

func idOf(t *testing.T, data []byte) string {
	var m struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &m); err != nil || m.ID == "" {
		t.Fatalf("no id in response: %s", data)
	}
	return m.ID
}

func TestAuthoringReviewPublishFlow(t *testing.T) {
	srv, app, pool := setup(t)
	ctx := context.Background()
	creator := "11111111-1111-1111-1111-111111111111"
	admin := "22222222-2222-2222-2222-222222222222"

	// Creator makes a show.
	resp, data := do(t, "POST", srv.URL+"/shows", map[string]any{
		"title": "The Vant6 Signal", "synopsis": "A dockworker hears a voice in the harbor noise.",
		"language_code": "en", "genre_ids": []string{}, "tags": []string{"mystery", "audio-drama"},
	}, hdr(creator, "CREATOR"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create show: %d %s", resp.StatusCode, data)
	}
	showID := idOf(t, data)

	// A plain USER cannot create shows.
	resp, _ = do(t, "POST", srv.URL+"/shows", map[string]any{"title": "x", "language_code": "en"}, hdr(creator, "USER"))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("USER create show: expected 403, got %d", resp.StatusCode)
	}

	// Season + episode.
	resp, data = do(t, "POST", srv.URL+"/shows/"+showID+"/seasons",
		map[string]any{"number": 1, "title": "Season 1"}, hdr(creator, "CREATOR"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create season: %d %s", resp.StatusCode, data)
	}
	seasonID := idOf(t, data)

	resp, data = do(t, "POST", srv.URL+"/seasons/"+seasonID+"/episodes",
		map[string]any{"number": 1, "title": "Harbor Lights", "synopsis": "The first transmission."},
		hdr(creator, "CREATOR"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create episode: %d %s", resp.StatusCode, data)
	}
	episodeID := idOf(t, data)

	// Simulate the media pipeline finishing: attach packaged audio.
	if err := app.Store.AttachMedia(ctx, episodeID, MediaMetadata{
		HLSMasterKey: "hls/" + episodeID + "/master.m3u8",
		Variants: []AudioVariant{
			{BitrateKbps: 64, Key: "a/64.m3u8", Codec: "aac"},
			{BitrateKbps: 128, Key: "a/128.m3u8", Codec: "aac"},
			{BitrateKbps: 256, Key: "a/256.m3u8", Codec: "aac"},
		},
		Codec: "aac", SampleRateHz: 44100, Channels: 2, DurationSec: 1420, FileSizeBytes: 5 << 20,
	}); err != nil {
		t.Fatalf("attach media: %v", err)
	}

	// Submit episode and show for review.
	if resp, data = do(t, "POST", srv.URL+"/episodes/"+episodeID+"/submit", nil, hdr(creator, "CREATOR")); resp.StatusCode != http.StatusOK {
		t.Fatalf("submit episode: %d %s", resp.StatusCode, data)
	}
	if resp, data = do(t, "POST", srv.URL+"/shows/"+showID+"/submit", nil, hdr(creator, "CREATOR")); resp.StatusCode != http.StatusOK {
		t.Fatalf("submit show: %d %s", resp.StatusCode, data)
	}

	// Creator cannot approve their own work.
	resp, _ = do(t, "POST", srv.URL+"/admin/review/show/"+showID,
		map[string]any{"action": "approve"}, hdr(creator, "CREATOR"))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("creator approve: expected 403, got %d", resp.StatusCode)
	}

	// Admin approves then publishes both.
	for _, target := range []string{"episode/" + episodeID, "show/" + showID} {
		if resp, data = do(t, "POST", srv.URL+"/admin/review/"+target,
			map[string]any{"action": "approve"}, hdr(admin, "ADMIN")); resp.StatusCode != http.StatusOK {
			t.Fatalf("approve %s: %d %s", target, resp.StatusCode, data)
		}
		if resp, data = do(t, "POST", srv.URL+"/admin/review/"+target,
			map[string]any{"action": "publish"}, hdr(admin, "ADMIN")); resp.StatusCode != http.StatusOK {
			t.Fatalf("publish %s: %d %s", target, resp.StatusCode, data)
		}
	}

	// The show is now in the public catalog and searchable.
	resp, data = do(t, "GET", srv.URL+"/shows?q=harbor+signal", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: %d %s", resp.StatusCode, data)
	}
	var list struct {
		Shows []Show `json:"shows"`
		Total int    `json:"total"`
	}
	_ = json.Unmarshal(data, &list)
	if list.Total < 1 {
		t.Fatalf("published show not found by search: %s", data)
	}
	if list.Shows[0].EpisodeCount != 1 {
		t.Fatalf("expected episode_count 1 after publish, got %d", list.Shows[0].EpisodeCount)
	}

	// Publication events were written to the outbox for downstream services.
	var events int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE envelope->>'event_type' LIKE 'content.%_published'`,
	).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("expected 2 publication events, got %d", events)
	}

	// The public episode projection hides the script and internal media keys.
	resp, data = do(t, "GET", srv.URL+"/episodes/"+episodeID, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get episode: %d %s", resp.StatusCode, data)
	}
	var ep Episode
	_ = json.Unmarshal(data, &ep)
	if ep.HLSMasterKey != "" {
		t.Fatal("public episode leaked the HLS master key")
	}
}

// TestShowReviewCascadesToReadyEpisodes covers the AI-generation path: the
// creator only ever acts on the show, and every episode with finished audio
// rides along through submit, approve and publish. Episodes still processing
// are left in draft.
func TestShowReviewCascadesToReadyEpisodes(t *testing.T) {
	srv, app, pool := setup(t)
	ctx := context.Background()
	creator := "33333333-3333-3333-3333-333333333333"
	admin := "44444444-4444-4444-4444-444444444444"

	_, data := do(t, "POST", srv.URL+"/shows", map[string]any{
		"title": "Tidewater", "synopsis": "Salt marsh field recordings turn into a story.",
		"language_code": "en", "genre_ids": []string{}, "tags": []string{"mystery"},
	}, hdr(creator, "CREATOR"))
	showID := idOf(t, data)

	_, data = do(t, "POST", srv.URL+"/shows/"+showID+"/seasons",
		map[string]any{"number": 1, "title": "Season 1"}, hdr(creator, "CREATOR"))
	seasonID := idOf(t, data)

	_, data = do(t, "POST", srv.URL+"/seasons/"+seasonID+"/episodes",
		map[string]any{"number": 1, "title": "Low Tide"}, hdr(creator, "CREATOR"))
	readyEp := idOf(t, data)
	_, data = do(t, "POST", srv.URL+"/seasons/"+seasonID+"/episodes",
		map[string]any{"number": 2, "title": "High Water"}, hdr(creator, "CREATOR"))
	pendingEp := idOf(t, data)

	// Only the first episode finishes rendering.
	if err := app.Store.AttachMedia(ctx, readyEp, MediaMetadata{
		HLSMasterKey: "hls/" + readyEp + "/master.m3u8",
		Variants: []AudioVariant{
			{BitrateKbps: 64, Key: "a/64.m3u8", Codec: "aac"},
			{BitrateKbps: 128, Key: "a/128.m3u8", Codec: "aac"},
			{BitrateKbps: 256, Key: "a/256.m3u8", Codec: "aac"},
		},
		Codec: "aac", SampleRateHz: 44100, Channels: 2, DurationSec: 1100, FileSizeBytes: 4 << 20,
	}); err != nil {
		t.Fatalf("attach media: %v", err)
	}

	// Creator submits only the show.
	if resp, d := do(t, "POST", srv.URL+"/shows/"+showID+"/submit", nil, hdr(creator, "CREATOR")); resp.StatusCode != http.StatusOK {
		t.Fatalf("submit show: %d %s", resp.StatusCode, d)
	}
	// Admin approves then publishes only the show.
	for _, action := range []string{"approve", "publish"} {
		if resp, d := do(t, "POST", srv.URL+"/admin/review/show/"+showID,
			map[string]any{"action": action}, hdr(admin, "ADMIN")); resp.StatusCode != http.StatusOK {
			t.Fatalf("%s show: %d %s", action, resp.StatusCode, d)
		}
	}

	ready, err := app.Store.EpisodeByID(ctx, readyEp)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != StatusPublished {
		t.Fatalf("ready episode should be published, got %q", ready.Status)
	}
	pending, err := app.Store.EpisodeByID(ctx, pendingEp)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != StatusDraft {
		t.Fatalf("unprocessed episode should stay draft, got %q", pending.Status)
	}

	sh, err := app.Store.ShowByID(ctx, showID)
	if err != nil {
		t.Fatal(err)
	}
	if sh.EpisodeCount != 1 {
		t.Fatalf("expected episode_count 1, got %d", sh.EpisodeCount)
	}

	var showEvents, epEvents int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE envelope->>'event_type' = 'content.show_published'`,
	).Scan(&showEvents); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE envelope->>'event_type' = 'content.episode_published'`,
	).Scan(&epEvents); err != nil {
		t.Fatal(err)
	}
	if showEvents != 1 || epEvents != 1 {
		t.Fatalf("expected 1 show + 1 episode publication event, got %d + %d", showEvents, epEvents)
	}
}

// TestInternalEpisodeEndpointRequiresServiceToken guards the fix for the
// access-control hole: /internal/episodes/{id} returns unpublished episodes with
// their script and media keys, so it must reject callers without the shared
// service token even though the gateway lets any authenticated user reach it.
func TestInternalEpisodeEndpointRequiresServiceToken(t *testing.T) {
	srv, _, _ := setup(t)
	svc := map[string]string{"X-Auralis-Service-Token": "svc-token"}

	resp, data := do(t, "POST", srv.URL+"/internal/authoring/shows", map[string]any{
		"creator_user_id": "55555555-5555-5555-5555-555555555555",
		"title":           "Sealed Room", "language_code": "en",
	}, svc)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create show: %d %s", resp.StatusCode, data)
	}
	showID := idOf(t, data)

	resp, data = do(t, "POST", srv.URL+"/internal/authoring/episodes", map[string]any{
		"show_id": showID, "number": 1, "title": "Draft One", "script": "SECRET SCRIPT",
	}, svc)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create episode: %d %s", resp.StatusCode, data)
	}
	episodeID := idOf(t, data)

	// No service token: rejected.
	if resp, _ = do(t, "GET", srv.URL+"/internal/episodes/"+episodeID, nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("internal episode without token: expected 401, got %d", resp.StatusCode)
	}

	// With the token: the unpublished episode and its script come back.
	resp, data = do(t, "GET", srv.URL+"/internal/episodes/"+episodeID, nil, svc)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("internal episode with token: %d %s", resp.StatusCode, data)
	}
	if !bytes.Contains(data, []byte("SECRET SCRIPT")) {
		t.Fatalf("internal episode should include the script: %s", data)
	}
}
