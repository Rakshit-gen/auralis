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
	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/migrate"
	"github.com/auralis/user/migrations"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const secret = "user-test-identity"

func dbURL() string {
	if v := os.Getenv("USER_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://localhost:5432/user_test?sslmode=disable"
}

func setup(t *testing.T) (*httptest.Server, *App) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL())
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		t.Skipf("user_test database not reachable (%v)", err)
	}
	if err := migrate.Run(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `TRUNCATE profiles, preferences, likes, bookmarks, follows,
		listening_history, entitlements, promo_redemptions, outbox_events, processed_events
		RESTART IDENTITY CASCADE`)
	_, _ = pool.Exec(ctx, `UPDATE promo_codes SET redeemed_count = 0`)
	t.Cleanup(pool.Close)

	app := NewApp(NewStore(pool), "svc")
	r := chi.NewRouter()
	r.Use(authn.ServiceMiddleware(secret))
	app.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, app
}

func hdr(userID string, roles ...string) map[string]string {
	return authn.IdentityHeaders(secret, authn.Identity{UserID: userID, Roles: roles}, "req")
}

func req(t *testing.T, method, url string, body any, h map[string]string) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	r, _ := http.NewRequest(method, url, &buf)
	r.Header.Set("Content-Type", "application/json")
	for k, v := range h {
		r.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

func mkEvent(t *testing.T, typ string, payload any) envelope.Envelope {
	t.Helper()
	e, err := envelope.New(typ, 1, "test", "corr", "", payload)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRegistrationThenLibraryFlow(t *testing.T) {
	srv, app := setup(t)
	ctx := context.Background()
	user := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	show := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	ep := "cccccccc-cccc-cccc-cccc-cccccccccccc"

	// The user.registered event provisions the profile, preferences, entitlement.
	if err := app.Handle(ctx, mkEvent(t, "user.registered", map[string]any{
		"user_id": user, "email": "reader@example.com", "display_name": "Reader",
	})); err != nil {
		t.Fatalf("handle registered: %v", err)
	}

	resp, data := req(t, "GET", srv.URL+"/me/profile", nil, hdr(user, "USER"))
	if resp.StatusCode != 200 {
		t.Fatalf("get profile: %d %s", resp.StatusCode, data)
	}

	// Free plan by default.
	resp, data = req(t, "GET", srv.URL+"/me/entitlement", nil, hdr(user, "USER"))
	var ent map[string]any
	_ = json.Unmarshal(data, &ent)
	if ent["plan"] != "free" || ent["premium_active"] != false {
		t.Fatalf("expected free plan: %s", data)
	}

	// Redeem the seeded promo code: now premium.
	resp, data = req(t, "POST", srv.URL+"/me/entitlement/redeem", map[string]string{"code": "auralis-premium"}, hdr(user, "USER"))
	if resp.StatusCode != 200 {
		t.Fatalf("redeem: %d %s", resp.StatusCode, data)
	}
	_ = json.Unmarshal(data, &ent)
	if ent["plan"] != "premium" || ent["premium_active"] != true {
		t.Fatalf("expected premium after redeem: %s", data)
	}

	// Second redemption by the same user is rejected.
	resp, _ = req(t, "POST", srv.URL+"/me/entitlement/redeem", map[string]string{"code": "AURALIS-PREMIUM"}, hdr(user, "USER"))
	if resp.StatusCode != 400 {
		t.Fatalf("double redeem: expected 400, got %d", resp.StatusCode)
	}

	// Like a show and confirm it lists back.
	resp, _ = req(t, "POST", srv.URL+"/me/likes", map[string]string{"type": "show", "id": show, "show_id": show}, hdr(user, "USER"))
	if resp.StatusCode != 200 {
		t.Fatalf("add like: %d", resp.StatusCode)
	}
	resp, data = req(t, "GET", srv.URL+"/me/likes", nil, hdr(user, "USER"))
	var likes struct {
		Likes []Like `json:"likes"`
	}
	_ = json.Unmarshal(data, &likes)
	if len(likes.Likes) != 1 || likes.Likes[0].TargetID != show {
		t.Fatalf("like not returned: %s", data)
	}

	// Out-of-order playback events must not regress listened_sec.
	now := time.Now()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(app.Handle(ctx, mkEvent(t, "playback.progress", map[string]any{
		"user_id": user, "episode_id": ep, "show_id": show, "position_sec": 900, "occurred_at": now,
	})))
	must(app.Handle(ctx, mkEvent(t, "playback.progress", map[string]any{
		"user_id": user, "episode_id": ep, "show_id": show, "position_sec": 300, "occurred_at": now.Add(-time.Minute),
	})))
	must(app.Handle(ctx, mkEvent(t, "playback.completed", map[string]any{
		"user_id": user, "episode_id": ep, "show_id": show, "position_sec": 1200, "occurred_at": now.Add(time.Minute),
	})))

	resp, data = req(t, "GET", srv.URL+"/me/history", nil, hdr(user, "USER"))
	var hist struct {
		History []HistoryEntry `json:"history"`
	}
	_ = json.Unmarshal(data, &hist)
	if len(hist.History) != 1 {
		t.Fatalf("expected one history row: %s", data)
	}
	if hist.History[0].ListenedSec != 1200 || !hist.History[0].Completed {
		t.Fatalf("history did not merge correctly: %+v", hist.History[0])
	}

	// Anonymous access to /me is rejected.
	resp, _ = req(t, "GET", srv.URL+"/me/profile", nil, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("anon /me: expected 401, got %d", resp.StatusCode)
	}
}
