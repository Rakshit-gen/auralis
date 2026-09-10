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
	"time"

	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/migrate"
	"github.com/auralis/platform/svcclient"
	"github.com/auralis/playback/migrations"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const secret = "playback-test-identity"

func dbURL() string {
	if v := os.Getenv("PLAYBACK_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://localhost:5432/playback_test?sslmode=disable"
}

// stubContent serves a single published, non-premium episode with media.
func stubContent(episodeID, showID string, premium bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"episode": map[string]any{
				"id": episodeID, "show_id": showID, "title": "Harbor Lights", "number": 1,
				"status": "published", "is_premium": premium, "free_preview_sec": 60,
				"duration_sec": 1400, "hls_master_key": "hls/" + episodeID + "/master.m3u8",
				"audio_variants": []map[string]any{
					{"bitrate_kbps": 64, "key": "a/64.m3u8", "codec": "aac"},
					{"bitrate_kbps": 128, "key": "a/128.m3u8", "codec": "aac"},
					{"bitrate_kbps": 256, "key": "a/256.m3u8", "codec": "aac"},
				},
			},
			"show": map[string]any{"slug": "harbor-lights", "title": "Harbor Lights"},
		})
	}))
}

func stubUsers(premiumActive bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"premium_active": premiumActive})
	}))
}

// fakeSigner returns a deterministic signed URL without contacting storage.
type fakeSigner struct{}

func (fakeSigner) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://media.test/" + key + "?sig=stub", nil
}

func (fakeSigner) PublicURL(string) string { return "" }

func setup(t *testing.T, content, users *httptest.Server) (*httptest.Server, *App, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL())
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		t.Skipf("playback_test database not reachable (%v)", err)
	}
	if err := migrate.Run(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `TRUNCATE episode_cache, devices, playback_sessions, playback_progress,
		seen_events, outbox_events, processed_events RESTART IDENTITY CASCADE`)
	t.Cleanup(pool.Close)

	app := NewApp(NewStore(pool), fakeSigner{},
		svcclient.New(content.URL, "svc", "playback"),
		svcclient.New(users.URL, "svc", "playback"))
	r := chi.NewRouter()
	r.Use(authn.ServiceMiddleware(secret))
	app.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, app, pool
}

func hdr(u string) map[string]string {
	return authn.IdentityHeaders(secret, authn.Identity{UserID: u, Roles: []string{"USER"}}, "req")
}

func call(t *testing.T, method, url string, body any, h map[string]string) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range h {
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

func TestProgressOrderingAndIdempotency(t *testing.T) {
	ep := "10000000-0000-0000-0000-000000000001"
	show := "20000000-0000-0000-0000-000000000001"
	user := "30000000-0000-0000-0000-000000000001"

	content := stubContent(ep, show, false)
	defer content.Close()
	users := stubUsers(false)
	defer users.Close()

	srv, _, pool := setup(t, content, users)
	ctx := context.Background()

	now := time.Now().UTC()

	// First progress at 600s.
	resp, data := call(t, "POST", srv.URL+"/playback/progress", map[string]any{
		"episode_id": ep, "show_id": show, "position_sec": 600, "duration_sec": 1400,
		"client_event_id": "evt-1", "occurred_at": now.Format(time.RFC3339),
	}, hdr(user))
	if resp.StatusCode != 200 {
		t.Fatalf("progress 1: %d %s", resp.StatusCode, data)
	}

	// An older event arriving late with a lower position must not roll back.
	call(t, "POST", srv.URL+"/playback/progress", map[string]any{
		"episode_id": ep, "show_id": show, "position_sec": 120,
		"client_event_id": "evt-2", "occurred_at": now.Add(-2 * time.Minute).Format(time.RFC3339),
	}, hdr(user))

	var p Progress
	call(t, "GET", srv.URL+"/playback/progress/"+ep, nil, hdr(user))
	_, data = call(t, "GET", srv.URL+"/playback/progress/"+ep, nil, hdr(user))
	_ = json.Unmarshal(data, &p)
	if p.PositionSec != 600 {
		t.Fatalf("out-of-order event rolled back progress to %d", p.PositionSec)
	}

	// A newer event advances it.
	call(t, "POST", srv.URL+"/playback/progress", map[string]any{
		"episode_id": ep, "show_id": show, "position_sec": 900,
		"client_event_id": "evt-3", "occurred_at": now.Add(time.Minute).Format(time.RFC3339),
	}, hdr(user))
	_, data = call(t, "GET", srv.URL+"/playback/progress/"+ep, nil, hdr(user))
	_ = json.Unmarshal(data, &p)
	if p.PositionSec != 900 {
		t.Fatalf("newer event did not advance progress, got %d", p.PositionSec)
	}

	// Replaying evt-3 is deduped: no extra Kafka event, stored progress unchanged.
	resp, _ = call(t, "POST", srv.URL+"/playback/progress", map[string]any{
		"episode_id": ep, "show_id": show, "position_sec": 900,
		"client_event_id": "evt-3", "occurred_at": now.Add(time.Minute).Format(time.RFC3339),
	}, hdr(user))
	if resp.StatusCode != 200 {
		t.Fatalf("replay: %d", resp.StatusCode)
	}

	// evt-1, evt-2, evt-3 each produced one event; the evt-3 replay produced none.
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 3 {
		t.Fatalf("expected 3 outbox events (duplicate suppressed), got %d", events)
	}
}

// TestProgressColdCacheEnrichesFromContent covers the cold-cache path: the
// episode is not in episode_cache and the client sends only episode_id and a
// position. The service must fall back to the content lookup so the stored
// progress and the analytics event carry a real show_id and duration.
func TestProgressColdCacheEnrichesFromContent(t *testing.T) {
	ep := "10000000-0000-0000-0000-000000000009"
	show := "20000000-0000-0000-0000-000000000009"
	user := "30000000-0000-0000-0000-000000000009"

	content := stubContent(ep, show, false)
	defer content.Close()
	users := stubUsers(false)
	defer users.Close()

	srv, _, _ := setup(t, content, users) // setup truncates episode_cache

	resp, data := call(t, "POST", srv.URL+"/playback/progress", map[string]any{
		"episode_id": ep, "position_sec": 300, "client_event_id": "cold-1",
	}, hdr(user))
	if resp.StatusCode != 200 {
		t.Fatalf("progress: %d %s", resp.StatusCode, data)
	}

	var p Progress
	_, data = call(t, "GET", srv.URL+"/playback/progress/"+ep, nil, hdr(user))
	_ = json.Unmarshal(data, &p)
	if p.ShowID != show {
		t.Fatalf("show_id not enriched from content: %q", p.ShowID)
	}
	if p.DurationSec != 1400 {
		t.Fatalf("duration_sec not enriched from content, got %d", p.DurationSec)
	}
}

func TestAuthorizePremiumGate(t *testing.T) {
	ep := "10000000-0000-0000-0000-000000000002"
	show := "20000000-0000-0000-0000-000000000002"
	user := "30000000-0000-0000-0000-000000000002"

	content := stubContent(ep, show, true) // premium episode
	defer content.Close()
	users := stubUsers(false) // no premium entitlement
	defer users.Close()

	srv, _, _ := setup(t, content, users)

	// Free user gets preview-only access (episode has a 60s preview).
	resp, data := call(t, "POST", srv.URL+"/playback/authorize", map[string]any{"episode_id": ep}, hdr(user))
	if resp.StatusCode != 200 {
		t.Fatalf("authorize preview: %d %s", resp.StatusCode, data)
	}
	var auth map[string]any
	_ = json.Unmarshal(data, &auth)
	if auth["preview_only"] != true || auth["preview_limit_sec"].(float64) != 60 {
		t.Fatalf("expected preview-only access: %s", data)
	}
	if auth["hls_master_url"] == "" {
		t.Fatal("expected a signed master url even for preview")
	}
}
