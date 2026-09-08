package internal

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }
func (s *Store) Pool() *pgxpool.Pool     { return s.pool }

// AudioVariant mirrors the content service's rendition metadata.
type AudioVariant struct {
	BitrateKbps int    `json:"bitrate_kbps"`
	Key         string `json:"key"`
	Codec       string `json:"codec"`
	SizeBytes   int64  `json:"size_bytes"`
}

// EpisodeMeta is the cached authorization-relevant metadata for an episode.
type EpisodeMeta struct {
	EpisodeID      string         `json:"episode_id"`
	ShowID         string         `json:"show_id"`
	ShowSlug       string         `json:"show_slug"`
	ShowTitle      string         `json:"show_title"`
	EpisodeTitle   string         `json:"episode_title"`
	EpisodeNumber  int            `json:"episode_number"`
	DurationSec    int            `json:"duration_sec"`
	IsPremium      bool           `json:"is_premium"`
	FreePreviewSec int            `json:"free_preview_sec"`
	HLSMasterKey   string         `json:"hls_master_key"`
	AudioVariants  []AudioVariant `json:"audio_variants"`
	Published      bool           `json:"published"`
}

// Progress is a user's position in an episode.
type Progress struct {
	EpisodeID   string    `json:"episode_id"`
	ShowID      string    `json:"show_id"`
	PositionSec int       `json:"position_sec"`
	DurationSec int       `json:"duration_sec"`
	Completed   bool      `json:"completed"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Store) UpsertEpisodeCache(ctx context.Context, m EpisodeMeta) error {
	variants, _ := json.Marshal(m.AudioVariants)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO episode_cache (episode_id, show_id, show_slug, show_title, episode_title,
			episode_number, duration_sec, is_premium, free_preview_sec, hls_master_key, audio_variants, published)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 ON CONFLICT (episode_id) DO UPDATE SET
			show_id = EXCLUDED.show_id, show_slug = EXCLUDED.show_slug, show_title = EXCLUDED.show_title,
			episode_title = EXCLUDED.episode_title, episode_number = EXCLUDED.episode_number,
			duration_sec = EXCLUDED.duration_sec, is_premium = EXCLUDED.is_premium,
			free_preview_sec = EXCLUDED.free_preview_sec, hls_master_key = EXCLUDED.hls_master_key,
			audio_variants = EXCLUDED.audio_variants, published = EXCLUDED.published, updated_at = now()`,
		m.EpisodeID, m.ShowID, m.ShowSlug, m.ShowTitle, m.EpisodeTitle, m.EpisodeNumber,
		m.DurationSec, m.IsPremium, m.FreePreviewSec, m.HLSMasterKey, variants, m.Published)
	return err
}

func (s *Store) SetEpisodePublished(ctx context.Context, episodeID string, published bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE episode_cache SET published = $2, updated_at = now() WHERE episode_id = $1`,
		episodeID, published)
	return err
}

func (s *Store) EpisodeMeta(ctx context.Context, episodeID string) (EpisodeMeta, error) {
	var m EpisodeMeta
	var variants []byte
	err := s.pool.QueryRow(ctx,
		`SELECT episode_id, show_id, show_slug, show_title, episode_title, episode_number,
			duration_sec, is_premium, free_preview_sec, hls_master_key, audio_variants, published
		 FROM episode_cache WHERE episode_id = $1`, episodeID).
		Scan(&m.EpisodeID, &m.ShowID, &m.ShowSlug, &m.ShowTitle, &m.EpisodeTitle, &m.EpisodeNumber,
			&m.DurationSec, &m.IsPremium, &m.FreePreviewSec, &m.HLSMasterKey, &variants, &m.Published)
	if errors.Is(err, pgx.ErrNoRows) {
		return EpisodeMeta{}, ErrNotFound
	}
	if err != nil {
		return EpisodeMeta{}, err
	}
	_ = json.Unmarshal(variants, &m.AudioVariants)
	return m, nil
}

// --- progress ---

// SaveProgress applies a progress observation. It returns the resulting stored
// progress. An out-of-order event (occurredAt older than the stored
// last_event_at) does not move position backwards, but a larger position always
// wins so a fast client that briefly lost ordering still makes progress.
func (s *Store) SaveProgress(ctx context.Context, tx pgx.Tx, userID, episodeID, showID string,
	positionSec, durationSec int, completed bool, occurredAt time.Time) (Progress, error) {

	var p Progress
	err := tx.QueryRow(ctx,
		`INSERT INTO playback_progress (user_id, episode_id, show_id, position_sec, duration_sec, completed, last_event_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (user_id, episode_id) DO UPDATE SET
			position_sec = CASE
				WHEN $4 > playback_progress.position_sec THEN $4
				WHEN $7 >= playback_progress.last_event_at THEN $4
				ELSE playback_progress.position_sec END,
			duration_sec = GREATEST(playback_progress.duration_sec, $5),
			completed = playback_progress.completed OR $6,
			last_event_at = GREATEST(playback_progress.last_event_at, $7),
			updated_at = now()
		 RETURNING episode_id, show_id, position_sec, duration_sec, completed, updated_at`,
		userID, episodeID, showID, positionSec, durationSec, completed, occurredAt).
		Scan(&p.EpisodeID, &p.ShowID, &p.PositionSec, &p.DurationSec, &p.Completed, &p.UpdatedAt)
	return p, err
}

func (s *Store) Progress(ctx context.Context, userID, episodeID string) (Progress, error) {
	var p Progress
	err := s.pool.QueryRow(ctx,
		`SELECT episode_id, show_id, position_sec, duration_sec, completed, updated_at
		 FROM playback_progress WHERE user_id = $1 AND episode_id = $2`, userID, episodeID).
		Scan(&p.EpisodeID, &p.ShowID, &p.PositionSec, &p.DurationSec, &p.Completed, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Progress{EpisodeID: episodeID}, ErrNotFound
	}
	return p, err
}

// ContinueListening returns in-progress, not-completed episodes, newest first.
func (s *Store) ContinueListening(ctx context.Context, userID string, limit int) ([]Progress, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT episode_id, show_id, position_sec, duration_sec, completed, updated_at
		 FROM playback_progress
		 WHERE user_id = $1 AND completed = false AND position_sec > 0
		 ORDER BY updated_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Progress
	for rows.Next() {
		var p Progress
		if err := rows.Scan(&p.EpisodeID, &p.ShowID, &p.PositionSec, &p.DurationSec, &p.Completed, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- sessions and devices ---

func (s *Store) OpenSession(ctx context.Context, userID, episodeID, showID string, deviceID *string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO playback_sessions (user_id, episode_id, show_id, device_id)
		 VALUES ($1,$2,$3,$4) RETURNING id`, userID, episodeID, showID, deviceID).Scan(&id)
	return id, err
}

func (s *Store) TouchSession(ctx context.Context, sessionID, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE playback_sessions SET last_seen_at = now() WHERE id = $1 AND user_id = $2`,
		sessionID, userID)
	return err
}

func (s *Store) UpsertDevice(ctx context.Context, userID, deviceID, name, kind string) (string, error) {
	if deviceID == "" {
		var id string
		err := s.pool.QueryRow(ctx,
			`INSERT INTO devices (user_id, name, kind) VALUES ($1,$2,$3) RETURNING id`,
			userID, name, kind).Scan(&id)
		return id, err
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO devices (id, user_id, name, kind) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, kind = EXCLUDED.kind, last_seen_at = now()`,
		deviceID, userID, name, kind)
	return deviceID, err
}

func (s *Store) Devices(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, kind, last_seen_at, created_at FROM devices WHERE user_id = $1 ORDER BY last_seen_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, kind string
		var lastSeen, created time.Time
		if err := rows.Scan(&id, &name, &kind, &lastSeen, &created); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "kind": kind, "last_seen_at": lastSeen, "created_at": created,
		})
	}
	return out, rows.Err()
}

// --- event idempotency ---

// MarkSeen records a client event id and reports whether it is new.
func (s *Store) MarkSeen(ctx context.Context, tx pgx.Tx, clientEventID, userID string) (bool, error) {
	ct, err := tx.Exec(ctx,
		`INSERT INTO seen_events (client_event_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		clientEventID, userID)
	return ct.RowsAffected() > 0, err
}

func (s *Store) AlreadyProcessed(ctx context.Context, consumer, eventID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM processed_events WHERE consumer = $1 AND event_id = $2)`,
		consumer, eventID).Scan(&exists)
	return exists, err
}

func (s *Store) MarkProcessed(ctx context.Context, consumer, eventID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO processed_events (consumer, event_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		consumer, eventID)
	return err
}
