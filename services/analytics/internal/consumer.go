package internal

import (
	"context"
	"time"

	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/telemetry"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConsumerGroup is the analytics service's Kafka consumer group.
const ConsumerGroup = "analytics-service"

// Aggregator consumes domain events and maintains the aggregate tables.
type Aggregator struct {
	pool *pgxpool.Pool
}

func NewAggregator(pool *pgxpool.Pool) *Aggregator { return &Aggregator{pool: pool} }

// Handle applies one event. The processed-events insert and every aggregate
// update happen in one transaction, so a redelivered event is a no-op.
func (a *Aggregator) Handle(ctx context.Context, env envelope.Envelope) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	ct, err := tx.Exec(ctx,
		`INSERT INTO processed_events (consumer, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		ConsumerGroup, env.EventID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		telemetry.Count("analytics", "consume", "duplicate")
		return tx.Commit(ctx)
	}

	if err := a.apply(ctx, tx, env); err != nil {
		return err
	}
	telemetry.Count("analytics", "consume", "applied")
	return tx.Commit(ctx)
}

func isoWeek(t time.Time) time.Time {
	t = t.UTC()
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := t.AddDate(0, 0, -(weekday - 1))
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}

func (a *Aggregator) apply(ctx context.Context, tx pgx.Tx, env envelope.Envelope) error {
	switch env.EventType {
	case "user.registered":
		var p struct {
			UserID       string    `json:"user_id"`
			RegisteredAt time.Time `json:"registered_at"`
		}
		if err := env.Decode(&p); err != nil || p.UserID == "" {
			return err
		}
		at := p.RegisteredAt
		if at.IsZero() {
			at = env.OccurredAt
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO user_cohorts (user_id, cohort_week) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			p.UserID, isoWeek(at))
		return err

	case "content.show_published":
		var p struct {
			ShowID string `json:"show_id"`
			Title  string `json:"title"`
		}
		if err := env.Decode(&p); err != nil || p.ShowID == "" {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO show_stats (show_id, title) VALUES ($1, $2)
			 ON CONFLICT (show_id) DO UPDATE SET title = EXCLUDED.title`, p.ShowID, p.Title)
		return err

	case "user.liked", "user.unliked", "user.bookmarked", "user.followed", "user.unfollowed":
		return a.applySocial(ctx, tx, env)

	case "playback.progress", "playback.completed", "playback.play", "playback.skip",
		"playback.buffer_start", "playback.buffer_end", "playback.pause", "playback.seek":
		return a.applyPlayback(ctx, tx, env)

	default:
		logging.L(ctx).Debug("analytics ignoring event", "event_type", env.EventType)
		return nil
	}
}

func (a *Aggregator) applySocial(ctx context.Context, tx pgx.Tx, env envelope.Envelope) error {
	var p struct {
		ShowID string `json:"show_id"`
	}
	if err := env.Decode(&p); err != nil || p.ShowID == "" {
		return err
	}
	col, delta := "", 1
	switch env.EventType {
	case "user.liked":
		col = "likes"
	case "user.unliked":
		col, delta = "likes", -1
	case "user.bookmarked":
		col = "bookmarks"
	case "user.followed":
		col = "follows"
	case "user.unfollowed":
		col, delta = "follows", -1
	default:
		return nil
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO show_stats (show_id, `+col+`) VALUES ($1, GREATEST($2, 0))
		 ON CONFLICT (show_id) DO UPDATE SET `+col+` = GREATEST(show_stats.`+col+` + $2, 0), updated_at = now()`,
		p.ShowID, delta)
	return err
}

func (a *Aggregator) applyPlayback(ctx context.Context, tx pgx.Tx, env envelope.Envelope) error {
	var p struct {
		UserID      string    `json:"user_id"`
		EpisodeID   string    `json:"episode_id"`
		ShowID      string    `json:"show_id"`
		PositionSec int       `json:"position_sec"`
		DurationSec int       `json:"duration_sec"`
		Completed   bool      `json:"completed"`
		OccurredAt  time.Time `json:"occurred_at"`
	}
	if err := env.Decode(&p); err != nil {
		return err
	}
	if p.UserID == "" {
		return nil
	}
	at := p.OccurredAt
	if at.IsZero() {
		at = env.OccurredAt
	}
	day := at.UTC().Truncate(24 * time.Hour)
	week := isoWeek(at)

	// Active user for the day.
	if _, err := tx.Exec(ctx,
		`INSERT INTO listener_days (day, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		day, p.UserID); err != nil {
		return err
	}
	// Retention: link the user's cohort week to this activity week.
	if _, err := tx.Exec(ctx,
		`INSERT INTO retention (cohort_week, activity_week, user_id)
		 SELECT cohort_week, $2, user_id FROM user_cohorts WHERE user_id = $1
		 ON CONFLICT DO NOTHING`, p.UserID, week); err != nil {
		return err
	}

	isPlay := env.EventType == "playback.play"
	isProgress := env.EventType == "playback.progress" || env.EventType == "playback.completed"
	isSkip := env.EventType == "playback.skip"
	isBuffer := env.EventType == "playback.buffer_start"
	isComplete := env.EventType == "playback.completed" || p.Completed

	// Daily totals. listening_seconds is credited on completed events using the
	// episode duration, the one reliable figure that does not require per-episode
	// running state to avoid double counting from repeated progress events.
	_ = isProgress
	if _, err := tx.Exec(ctx,
		`INSERT INTO daily_totals (day, listening_seconds, plays, completes, skips, buffer_events)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (day) DO UPDATE SET
			listening_seconds = daily_totals.listening_seconds + EXCLUDED.listening_seconds,
			plays = daily_totals.plays + EXCLUDED.plays,
			completes = daily_totals.completes + EXCLUDED.completes,
			skips = daily_totals.skips + EXCLUDED.skips,
			buffer_events = daily_totals.buffer_events + EXCLUDED.buffer_events`,
		day, addSecondsOnComplete(isComplete, p.DurationSec), boolToInt(isPlay), boolToInt(isComplete),
		boolToInt(isSkip), boolToInt(isBuffer)); err != nil {
		return err
	}

	if p.EpisodeID != "" {
		ratio := 0.0
		if p.DurationSec > 0 {
			ratio = float64(p.PositionSec) / float64(p.DurationSec)
			if ratio > 1 {
				ratio = 1
			}
		}
		var ratioAdd float64
		var ratioN int
		if isComplete {
			ratioAdd, ratioN = ratio, 1
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO episode_stats (episode_id, show_id, plays, completes, skips, listening_seconds,
				completion_ratio_sum, completion_ratio_n)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			 ON CONFLICT (episode_id) DO UPDATE SET
				plays = episode_stats.plays + EXCLUDED.plays,
				completes = episode_stats.completes + EXCLUDED.completes,
				skips = episode_stats.skips + EXCLUDED.skips,
				listening_seconds = episode_stats.listening_seconds + EXCLUDED.listening_seconds,
				completion_ratio_sum = episode_stats.completion_ratio_sum + EXCLUDED.completion_ratio_sum,
				completion_ratio_n = episode_stats.completion_ratio_n + EXCLUDED.completion_ratio_n,
				updated_at = now()`,
			p.EpisodeID, p.ShowID, boolToInt(isPlay), boolToInt(isComplete), boolToInt(isSkip),
			addSecondsOnComplete(isComplete, p.DurationSec), ratioAdd, ratioN); err != nil {
			return err
		}
	}

	if p.ShowID != "" {
		if _, err := tx.Exec(ctx,
			`INSERT INTO show_listeners (show_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			p.ShowID, p.UserID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO show_stats (show_id, plays, completes, listening_seconds)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (show_id) DO UPDATE SET
				plays = show_stats.plays + EXCLUDED.plays,
				completes = show_stats.completes + EXCLUDED.completes,
				listening_seconds = show_stats.listening_seconds + EXCLUDED.listening_seconds,
				updated_at = now()`,
			p.ShowID, boolToInt(isPlay), boolToInt(isComplete),
			addSecondsOnComplete(isComplete, p.DurationSec)); err != nil {
			return err
		}
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func addSecondsOnComplete(complete bool, duration int) int {
	if complete && duration > 0 {
		return duration
	}
	return 0
}
