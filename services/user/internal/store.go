package internal

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

// Store is the user service database.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }
func (s *Store) Pool() *pgxpool.Pool     { return s.pool }

type Profile struct {
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Bio         string    `json:"bio"`
	Email       string    `json:"email"`
	CreatedAt   time.Time `json:"created_at"`
}

type Preferences struct {
	UserID        string   `json:"user_id"`
	GenreSlugs    []string `json:"genre_slugs"`
	LanguageCodes []string `json:"language_codes"`
	Autoplay      bool     `json:"autoplay"`
	PlaybackSpeed float64  `json:"playback_speed"`
	ExplicitOK    bool     `json:"explicit_ok"`
	EmailUpdates  bool     `json:"email_updates"`
}

type Entitlement struct {
	UserID    string     `json:"user_id"`
	Plan      string     `json:"plan"`
	Status    string     `json:"status"`
	Source    string     `json:"source"`
	GrantedAt time.Time  `json:"granted_at"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Active reports whether the entitlement currently grants premium access.
func (e Entitlement) Active() bool {
	if e.Plan != "premium" || e.Status != "active" {
		return false
	}
	return e.ExpiresAt == nil || e.ExpiresAt.After(time.Now())
}

type Like struct {
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	ShowID     string    `json:"show_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type Bookmark struct {
	EpisodeID string    `json:"episode_id"`
	ShowID    string    `json:"show_id"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type Follow struct {
	ShowID    string    `json:"show_id"`
	CreatedAt time.Time `json:"created_at"`
}

type HistoryEntry struct {
	EpisodeID   string    `json:"episode_id"`
	ShowID      string    `json:"show_id"`
	ListenedSec int       `json:"listened_sec"`
	Completed   bool      `json:"completed"`
	LastAt      time.Time `json:"last_at"`
}

// EnsureProfile creates the profile, default preferences, and default (free)
// entitlement for a newly registered user. Idempotent.
func (s *Store) EnsureProfile(ctx context.Context, userID, displayName, email string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`INSERT INTO profiles (user_id, display_name, email) VALUES ($1,$2,$3)
		 ON CONFLICT (user_id) DO UPDATE SET display_name = EXCLUDED.display_name, email = EXCLUDED.email`,
		userID, displayName, email); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO preferences (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO entitlements (user_id, plan, source) VALUES ($1,'free','default')
		 ON CONFLICT DO NOTHING`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Profile(ctx context.Context, userID string) (Profile, error) {
	var p Profile
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, display_name, avatar_url, bio, email, created_at FROM profiles WHERE user_id = $1`,
		userID).Scan(&p.UserID, &p.DisplayName, &p.AvatarURL, &p.Bio, &p.Email, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	return p, err
}

func (s *Store) UpdateProfile(ctx context.Context, userID, displayName, avatarURL, bio string) (Profile, error) {
	var p Profile
	err := s.pool.QueryRow(ctx,
		`UPDATE profiles SET display_name = $2, avatar_url = $3, bio = $4, updated_at = now()
		 WHERE user_id = $1
		 RETURNING user_id, display_name, avatar_url, bio, email, created_at`,
		userID, displayName, avatarURL, bio).
		Scan(&p.UserID, &p.DisplayName, &p.AvatarURL, &p.Bio, &p.Email, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	return p, err
}

func (s *Store) Preferences(ctx context.Context, userID string) (Preferences, error) {
	var p Preferences
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, genre_slugs, language_codes, autoplay, playback_speed, explicit_ok, email_updates
		 FROM preferences WHERE user_id = $1`, userID).
		Scan(&p.UserID, &p.GenreSlugs, &p.LanguageCodes, &p.Autoplay, &p.PlaybackSpeed, &p.ExplicitOK, &p.EmailUpdates)
	if errors.Is(err, pgx.ErrNoRows) {
		return Preferences{}, ErrNotFound
	}
	if p.GenreSlugs == nil {
		p.GenreSlugs = []string{}
	}
	if p.LanguageCodes == nil {
		p.LanguageCodes = []string{}
	}
	return p, err
}

func (s *Store) UpdatePreferences(ctx context.Context, p Preferences) (Preferences, error) {
	err := s.pool.QueryRow(ctx,
		`UPDATE preferences SET genre_slugs = $2, language_codes = $3, autoplay = $4,
			playback_speed = $5, explicit_ok = $6, email_updates = $7, updated_at = now()
		 WHERE user_id = $1
		 RETURNING user_id, genre_slugs, language_codes, autoplay, playback_speed, explicit_ok, email_updates`,
		p.UserID, p.GenreSlugs, p.LanguageCodes, p.Autoplay, p.PlaybackSpeed, p.ExplicitOK, p.EmailUpdates).
		Scan(&p.UserID, &p.GenreSlugs, &p.LanguageCodes, &p.Autoplay, &p.PlaybackSpeed, &p.ExplicitOK, &p.EmailUpdates)
	return p, err
}

// --- likes / bookmarks / follows ---

func (s *Store) AddLike(ctx context.Context, userID, targetType, targetID, showID string) (bool, error) {
	ct, err := s.pool.Exec(ctx,
		`INSERT INTO likes (user_id, target_type, target_id, show_id) VALUES ($1,$2,$3,$4)
		 ON CONFLICT DO NOTHING`, userID, targetType, targetID, showID)
	return ct.RowsAffected() > 0, err
}

func (s *Store) RemoveLike(ctx context.Context, userID, targetType, targetID string) (bool, error) {
	ct, err := s.pool.Exec(ctx,
		`DELETE FROM likes WHERE user_id = $1 AND target_type = $2 AND target_id = $3`,
		userID, targetType, targetID)
	return ct.RowsAffected() > 0, err
}

func (s *Store) Likes(ctx context.Context, userID string, limit, offset int) ([]Like, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT target_type, target_id, show_id, created_at FROM likes
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Like
	for rows.Next() {
		var l Like
		if err := rows.Scan(&l.TargetType, &l.TargetID, &l.ShowID, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) AddBookmark(ctx context.Context, userID, episodeID, showID, note string) (bool, error) {
	ct, err := s.pool.Exec(ctx,
		`INSERT INTO bookmarks (user_id, episode_id, show_id, note) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (user_id, episode_id) DO UPDATE SET note = EXCLUDED.note`,
		userID, episodeID, showID, note)
	return ct.RowsAffected() > 0, err
}

func (s *Store) RemoveBookmark(ctx context.Context, userID, episodeID string) (bool, error) {
	ct, err := s.pool.Exec(ctx,
		`DELETE FROM bookmarks WHERE user_id = $1 AND episode_id = $2`, userID, episodeID)
	return ct.RowsAffected() > 0, err
}

func (s *Store) Bookmarks(ctx context.Context, userID string, limit, offset int) ([]Bookmark, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT episode_id, show_id, note, created_at FROM bookmarks
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.EpisodeID, &b.ShowID, &b.Note, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) AddFollow(ctx context.Context, userID, showID string) (bool, error) {
	ct, err := s.pool.Exec(ctx,
		`INSERT INTO follows (user_id, show_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, showID)
	return ct.RowsAffected() > 0, err
}

func (s *Store) RemoveFollow(ctx context.Context, userID, showID string) (bool, error) {
	ct, err := s.pool.Exec(ctx,
		`DELETE FROM follows WHERE user_id = $1 AND show_id = $2`, userID, showID)
	return ct.RowsAffected() > 0, err
}

func (s *Store) Follows(ctx context.Context, userID string) ([]Follow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT show_id, created_at FROM follows WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Follow
	for rows.Next() {
		var f Follow
		if err := rows.Scan(&f.ShowID, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// --- history ---

// RecordListening merges a playback observation into the history log. It never
// lowers listened_sec so out-of-order events do not regress progress.
func (s *Store) RecordListening(ctx context.Context, userID, episodeID, showID string, listenedSec int, completed bool, at time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO listening_history (user_id, episode_id, show_id, listened_sec, completed, first_at, last_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$6)
		 ON CONFLICT (user_id, episode_id) DO UPDATE SET
			listened_sec = GREATEST(listening_history.listened_sec, EXCLUDED.listened_sec),
			completed = listening_history.completed OR EXCLUDED.completed,
			last_at = GREATEST(listening_history.last_at, EXCLUDED.last_at)`,
		userID, episodeID, showID, listenedSec, completed, at)
	return err
}

func (s *Store) History(ctx context.Context, userID string, limit, offset int) ([]HistoryEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT episode_id, show_id, listened_sec, completed, last_at FROM listening_history
		 WHERE user_id = $1 ORDER BY last_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryEntry
	for rows.Next() {
		var h HistoryEntry
		if err := rows.Scan(&h.EpisodeID, &h.ShowID, &h.ListenedSec, &h.Completed, &h.LastAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// --- entitlements ---

func (s *Store) Entitlement(ctx context.Context, userID string) (Entitlement, error) {
	var e Entitlement
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, plan, status, source, granted_at, expires_at FROM entitlements WHERE user_id = $1`,
		userID).Scan(&e.UserID, &e.Plan, &e.Status, &e.Source, &e.GrantedAt, &e.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entitlement{UserID: userID, Plan: "free", Status: "active", Source: "default"}, nil
	}
	// Lazily expire.
	if e.Plan == "premium" && e.ExpiresAt != nil && e.ExpiresAt.Before(time.Now()) && e.Status == "active" {
		_, _ = s.pool.Exec(ctx, `UPDATE entitlements SET status = 'expired', updated_at = now() WHERE user_id = $1`, userID)
		e.Status = "expired"
	}
	return e, err
}

func (s *Store) SetEntitlement(ctx context.Context, tx pgx.Tx, userID, plan, source string, expires *time.Time) (Entitlement, error) {
	var e Entitlement
	err := tx.QueryRow(ctx,
		`INSERT INTO entitlements (user_id, plan, status, source, granted_at, expires_at)
		 VALUES ($1,$2,'active',$3, now(), $4)
		 ON CONFLICT (user_id) DO UPDATE SET
			plan = EXCLUDED.plan, status = 'active', source = EXCLUDED.source,
			granted_at = now(), expires_at = EXCLUDED.expires_at, updated_at = now()
		 RETURNING user_id, plan, status, source, granted_at, expires_at`,
		userID, plan, source, expires).
		Scan(&e.UserID, &e.Plan, &e.Status, &e.Source, &e.GrantedAt, &e.ExpiresAt)
	return e, err
}

// RedeemPromo applies a promo code to a user in one transaction, enforcing the
// per-user and global redemption limits.
func (s *Store) RedeemPromo(ctx context.Context, userID, code string) (Entitlement, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Entitlement{}, err
	}
	defer tx.Rollback(ctx)

	var plan string
	var durationDays, maxR, redeemed int
	var active bool
	err = tx.QueryRow(ctx,
		`SELECT plan, duration_days, max_redemptions, redeemed_count, active
		 FROM promo_codes WHERE code = $1 FOR UPDATE`, code).
		Scan(&plan, &durationDays, &maxR, &redeemed, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entitlement{}, ErrNotFound
	}
	if err != nil {
		return Entitlement{}, err
	}
	if !active || redeemed >= maxR {
		return Entitlement{}, errors.New("promo code is no longer available")
	}

	ct, err := tx.Exec(ctx,
		`INSERT INTO promo_redemptions (code, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, code, userID)
	if err != nil {
		return Entitlement{}, err
	}
	if ct.RowsAffected() == 0 {
		return Entitlement{}, errors.New("you have already redeemed this code")
	}
	if _, err := tx.Exec(ctx,
		`UPDATE promo_codes SET redeemed_count = redeemed_count + 1 WHERE code = $1`, code); err != nil {
		return Entitlement{}, err
	}

	expires := time.Now().AddDate(0, 0, durationDays)
	ent, err := s.SetEntitlement(ctx, tx, userID, plan, "promo_code", &expires)
	if err != nil {
		return Entitlement{}, err
	}
	return ent, tx.Commit(ctx)
}

// --- idempotency ---

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
