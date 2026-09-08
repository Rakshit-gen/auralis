package internal

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/auralis/analytics/migrations"
	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func dbURL() string {
	if v := os.Getenv("ANALYTICS_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://localhost:5432/analytics_test?sslmode=disable"
}

func newAgg(t *testing.T) (*Aggregator, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL())
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		t.Skipf("analytics_test database not reachable (%v)", err)
	}
	if err := migrate.Run(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `TRUNCATE processed_events, listener_days, daily_totals, episode_stats,
		show_stats, show_listeners, retention, user_cohorts RESTART IDENTITY CASCADE`)
	t.Cleanup(pool.Close)
	return NewAggregator(pool), pool
}

func ev(t *testing.T, typ string, payload any) envelope.Envelope {
	t.Helper()
	e, err := envelope.New(typ, 1, "test", "c", "", payload)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAggregationAndIdempotency(t *testing.T) {
	agg, pool := newAgg(t)
	ctx := context.Background()
	user := "aaaaaaaa-0000-0000-0000-000000000001"
	show := "bbbbbbbb-0000-0000-0000-000000000001"
	ep := "cccccccc-0000-0000-0000-000000000001"
	now := time.Now().UTC()

	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(agg.Handle(ctx, ev(t, "user.registered", map[string]any{"user_id": user, "registered_at": now})))
	must(agg.Handle(ctx, ev(t, "content.show_published", map[string]any{"show_id": show, "title": "Harbor Lights"})))

	play := ev(t, "playback.play", map[string]any{
		"user_id": user, "episode_id": ep, "show_id": show, "occurred_at": now,
	})
	must(agg.Handle(ctx, play))
	must(agg.Handle(ctx, play)) // redelivery: must be a no-op

	complete := ev(t, "playback.completed", map[string]any{
		"user_id": user, "episode_id": ep, "show_id": show,
		"position_sec": 1400, "duration_sec": 1400, "completed": true, "occurred_at": now,
	})
	must(agg.Handle(ctx, complete))
	must(agg.Handle(ctx, ev(t, "user.liked", map[string]any{"user_id": user, "show_id": show, "target_id": show})))

	var plays, completes, likes int64
	must(pool.QueryRow(ctx,
		`SELECT plays, completes, likes FROM show_stats WHERE show_id = $1`, show).Scan(&plays, &completes, &likes))
	if plays != 1 || completes != 1 || likes != 1 {
		t.Fatalf("show_stats wrong after redelivery: plays=%d completes=%d likes=%d", plays, completes, likes)
	}

	var dau int
	must(pool.QueryRow(ctx,
		`SELECT count(*) FROM listener_days WHERE day = $1`, now.Truncate(24*time.Hour)).Scan(&dau))
	if dau != 1 {
		t.Fatalf("expected DAU 1, got %d", dau)
	}

	var epPlays, epCompletes int64
	var ratioN int64
	must(pool.QueryRow(ctx,
		`SELECT plays, completes, completion_ratio_n FROM episode_stats WHERE episode_id = $1`, ep).
		Scan(&epPlays, &epCompletes, &ratioN))
	if epPlays != 1 || epCompletes != 1 || ratioN != 1 {
		t.Fatalf("episode_stats wrong: plays=%d completes=%d n=%d", epPlays, epCompletes, ratioN)
	}

	// Retention row links the cohort to this week's activity.
	var retentionRows int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM retention WHERE user_id = $1`, user).Scan(&retentionRows))
	if retentionRows != 1 {
		t.Fatalf("expected 1 retention row, got %d", retentionRows)
	}
}
