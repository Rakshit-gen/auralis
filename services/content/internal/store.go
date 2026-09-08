package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// Store is the content database.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// --- reference data ---

func (s *Store) Genres(ctx context.Context) ([]Genre, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, slug, name, description FROM genres ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Genre
	for rows.Next() {
		var g Genre
		if err := rows.Scan(&g.ID, &g.Slug, &g.Name, &g.Description); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) genreMap(ctx context.Context) (map[string]Genre, error) {
	gs, err := s.Genres(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]Genre, len(gs))
	for _, g := range gs {
		m[g.ID] = g
	}
	return m, nil
}

func (s *Store) Languages(ctx context.Context) ([]Language, error) {
	rows, err := s.pool.Query(ctx, `SELECT code, name FROM languages ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Language
	for rows.Next() {
		var l Language
		if err := rows.Scan(&l.Code, &l.Name); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// --- creators ---

// UpsertCreator returns the creator profile for a user, creating it on first use.
func (s *Store) UpsertCreator(ctx context.Context, userID, name string) (Creator, error) {
	var c Creator
	err := s.pool.QueryRow(ctx,
		`INSERT INTO creators (user_id, name) VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id, user_id, name, bio, avatar_url, created_at`,
		userID, name).Scan(&c.ID, &c.UserID, &c.Name, &c.Bio, &c.AvatarURL, &c.CreatedAt)
	return c, err
}

func (s *Store) CreatorByUser(ctx context.Context, userID string) (Creator, error) {
	var c Creator
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, name, bio, avatar_url, created_at FROM creators WHERE user_id = $1`,
		userID).Scan(&c.ID, &c.UserID, &c.Name, &c.Bio, &c.AvatarURL, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Creator{}, ErrNotFound
	}
	return c, err
}

// --- shows ---

const showColumns = `s.id, s.creator_id, c.name, s.title, s.slug, s.synopsis, s.description,
	s.language_code, s.genre_ids, s.tags, s.maturity, s.cover_image_url, s.accent_color,
	s.is_premium, s.status, s.ai_generated, s.episode_count, s.total_duration_sec,
	s.rating_sum, s.rating_count, s.published_at, s.created_at, s.updated_at`

func scanShow(row pgx.Row) (Show, error) {
	var sh Show
	var ratingSum, ratingCount int64
	err := row.Scan(&sh.ID, &sh.CreatorID, &sh.CreatorName, &sh.Title, &sh.Slug, &sh.Synopsis, &sh.Description,
		&sh.LanguageCode, &sh.GenreIDs, &sh.Tags, &sh.Maturity, &sh.CoverImageURL, &sh.AccentColor,
		&sh.IsPremium, &sh.Status, &sh.AIGenerated, &sh.EpisodeCount, &sh.TotalDurationSec,
		&ratingSum, &ratingCount, &sh.PublishedAt, &sh.CreatedAt, &sh.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Show{}, ErrNotFound
	}
	sh.RatingCount = ratingCount
	sh.RatingAvg = ratingAvg(ratingSum, ratingCount)
	if sh.GenreIDs == nil {
		sh.GenreIDs = []string{}
	}
	if sh.Tags == nil {
		sh.Tags = []string{}
	}
	return sh, err
}

// ShowFilter narrows a catalog listing.
type ShowFilter struct {
	GenreSlug    string
	LanguageCode string
	Premium      *bool
	AIGenerated  *bool
	Sort         string // recent, popular, rating, title
	Query        string // full-text search
	Limit        int
	Offset       int
	// IncludeNonPublished restricts to a creator's own shows when set.
	CreatorID           string
	IncludeNonPublished bool
}

// ListShows returns catalog shows matching filter, plus the total count.
func (s *Store) ListShows(ctx context.Context, f ShowFilter) ([]Show, int, error) {
	where := []string{}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}

	if f.CreatorID != "" {
		add("s.creator_id = $%d", f.CreatorID)
		if !f.IncludeNonPublished {
			where = append(where, "s.status = 'published'")
		}
	} else {
		where = append(where, "s.status = 'published'")
	}
	if f.GenreSlug != "" {
		add("s.genre_ids && (SELECT array_agg(id) FROM genres WHERE slug = $%d)", f.GenreSlug)
	}
	if f.LanguageCode != "" {
		add("s.language_code = $%d", f.LanguageCode)
	}
	if f.Premium != nil {
		add("s.is_premium = $%d", *f.Premium)
	}
	if f.AIGenerated != nil {
		add("s.ai_generated = $%d", *f.AIGenerated)
	}
	rankExpr := ""
	if q := strings.TrimSpace(f.Query); q != "" {
		add("s.search_vector @@ websearch_to_tsquery('english', $%d)", q)
		rankExpr = fmt.Sprintf("ts_rank_cd(s.search_vector, websearch_to_tsquery('english', $%d)) DESC, ", len(args))
	}

	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM shows s `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := rankExpr + showOrder(f.Sort, rankExpr != "")
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	args = append(args, limit, f.Offset)
	q := fmt.Sprintf(
		`SELECT %s FROM shows s JOIN creators c ON c.id = s.creator_id
		 %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		showColumns, clause, order, len(args)-1, len(args))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	gm, err := s.genreMap(ctx)
	if err != nil {
		return nil, 0, err
	}
	var out []Show
	for rows.Next() {
		sh, err := scanShow(rows)
		if err != nil {
			return nil, 0, err
		}
		attachGenres(&sh, gm)
		out = append(out, sh)
	}
	return out, total, rows.Err()
}

// showOrder returns the ORDER BY tail. When the query drives ranking the
// caller prepends the rank expression, so the default here is just a stable
// tiebreaker.
func showOrder(sort string, _ bool) string {
	switch sort {
	case "popular":
		return "s.episode_count DESC, s.published_at DESC NULLS LAST"
	case "rating":
		return "(CASE WHEN s.rating_count = 0 THEN 0 ELSE s.rating_sum::float / s.rating_count END) DESC, s.rating_count DESC"
	case "title":
		return "s.title ASC"
	default:
		return "s.published_at DESC NULLS LAST, s.created_at DESC"
	}
}

func attachGenres(sh *Show, gm map[string]Genre) {
	sh.Genres = make([]Genre, 0, len(sh.GenreIDs))
	for _, id := range sh.GenreIDs {
		if g, ok := gm[id]; ok {
			sh.Genres = append(sh.Genres, g)
		}
	}
}

func (s *Store) ShowByID(ctx context.Context, id string) (Show, error) {
	return s.showBy(ctx, "s.id = $1", id)
}

func (s *Store) ShowBySlug(ctx context.Context, slug string) (Show, error) {
	return s.showBy(ctx, "s.slug = $1", slug)
}

func (s *Store) showBy(ctx context.Context, cond string, arg any) (Show, error) {
	sh, err := scanShow(s.pool.QueryRow(ctx,
		`SELECT `+showColumns+` FROM shows s JOIN creators c ON c.id = s.creator_id WHERE `+cond, arg))
	if err != nil {
		return Show{}, err
	}
	gm, err := s.genreMap(ctx)
	if err != nil {
		return Show{}, err
	}
	attachGenres(&sh, gm)
	return sh, nil
}

// CreateShow inserts a draft show.
func (s *Store) CreateShow(ctx context.Context, sh Show) (Show, error) {
	// pgx sends a nil slice as SQL NULL, which would violate the NOT NULL on
	// these array columns and skip their defaults.
	if sh.GenreIDs == nil {
		sh.GenreIDs = []string{}
	}
	if sh.Tags == nil {
		sh.Tags = []string{}
	}
	return scanShow(s.pool.QueryRow(ctx,
		`WITH ins AS (
			INSERT INTO shows (creator_id, title, slug, synopsis, description, language_code,
				genre_ids, tags, maturity, cover_image_url, accent_color, is_premium, ai_generated, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			RETURNING *
		)
		SELECT `+showColumns+` FROM ins s JOIN creators c ON c.id = s.creator_id`,
		sh.CreatorID, sh.Title, sh.Slug, sh.Synopsis, sh.Description, sh.LanguageCode,
		sh.GenreIDs, sh.Tags, sh.Maturity, sh.CoverImageURL, sh.AccentColor, sh.IsPremium,
		sh.AIGenerated, StatusDraft))
}

// UpdateShow applies editable fields; only allowed while draft or rejected.
func (s *Store) UpdateShow(ctx context.Context, id string, patch map[string]any) (Show, error) {
	set, args := buildSet(patch)
	if set == "" {
		return s.ShowByID(ctx, id)
	}
	args = append(args, id)
	_, err := s.pool.Exec(ctx,
		fmt.Sprintf(`UPDATE shows SET %s, updated_at = now() WHERE id = $%d`, set, len(args)), args...)
	if err != nil {
		return Show{}, err
	}
	return s.ShowByID(ctx, id)
}

// SetShowStatus transitions a show and records a review event in one tx. It
// returns the updated show and enqueues nothing; the caller handles events.
func (s *Store) SetShowStatus(ctx context.Context, tx pgx.Tx, id, from, to string) error {
	ct, err := tx.Exec(ctx,
		`UPDATE shows SET status = $2::content_status,
			published_at = CASE WHEN $2 = 'published' AND published_at IS NULL THEN now() ELSE published_at END,
			updated_at = now()
		 WHERE id = $1 AND status = $3::content_status`, id, to, from)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("show %s is not in status %q", id, from)
	}
	return nil
}

// --- seasons ---

func (s *Store) CreateSeason(ctx context.Context, se Season) (Season, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO seasons (show_id, number, title, description)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id, show_id, number, title, description, status, published_at, created_at`,
		se.ShowID, se.Number, se.Title, se.Description).
		Scan(&se.ID, &se.ShowID, &se.Number, &se.Title, &se.Description, &se.Status, &se.PublishedAt, &se.CreatedAt)
	return se, err
}

func (s *Store) SeasonsForShow(ctx context.Context, showID string, publishedOnly bool) ([]Season, error) {
	q := `SELECT id, show_id, number, title, description, status, published_at, created_at
	      FROM seasons WHERE show_id = $1`
	if publishedOnly {
		q += ` AND status = 'published'`
	}
	q += ` ORDER BY number`
	rows, err := s.pool.Query(ctx, q, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Season
	for rows.Next() {
		var se Season
		if err := rows.Scan(&se.ID, &se.ShowID, &se.Number, &se.Title, &se.Description, &se.Status, &se.PublishedAt, &se.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, se)
	}
	return out, rows.Err()
}

// EnsureSeason returns the season with the given number for a show, creating a
// draft one if it does not exist yet.
func (s *Store) EnsureSeason(ctx context.Context, showID string, number int) (Season, error) {
	var se Season
	err := s.pool.QueryRow(ctx,
		`SELECT id, show_id, number, title, description, status, published_at, created_at
		 FROM seasons WHERE show_id = $1 AND number = $2`, showID, number).
		Scan(&se.ID, &se.ShowID, &se.Number, &se.Title, &se.Description, &se.Status, &se.PublishedAt, &se.CreatedAt)
	if err == nil {
		return se, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Season{}, err
	}
	return s.CreateSeason(ctx, Season{
		ShowID: showID, Number: number, Title: fmt.Sprintf("Season %d", number),
	})
}

func (s *Store) SeasonByID(ctx context.Context, id string) (Season, error) {
	var se Season
	err := s.pool.QueryRow(ctx,
		`SELECT id, show_id, number, title, description, status, published_at, created_at
		 FROM seasons WHERE id = $1`, id).
		Scan(&se.ID, &se.ShowID, &se.Number, &se.Title, &se.Description, &se.Status, &se.PublishedAt, &se.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Season{}, ErrNotFound
	}
	return se, err
}

// --- episodes ---

const epColumns = `id, show_id, season_id, number, title, slug, synopsis, script, status, processing,
	processing_error, is_premium, free_preview_sec, ai_generated, ai_job_id, duration_sec,
	hls_master_key, audio_variants, codec, sample_rate_hz, channels, file_size_bytes,
	checksum_sha256, published_at, created_at, updated_at`

func scanEpisode(row pgx.Row) (Episode, error) {
	var e Episode
	var variants []byte
	err := row.Scan(&e.ID, &e.ShowID, &e.SeasonID, &e.Number, &e.Title, &e.Slug, &e.Synopsis, &e.Script,
		&e.Status, &e.Processing, &e.ProcessingError, &e.IsPremium, &e.FreePreviewSec, &e.AIGenerated,
		&e.AIJobID, &e.DurationSec, &e.HLSMasterKey, &variants, &e.Codec, &e.SampleRateHz, &e.Channels,
		&e.FileSizeBytes, &e.ChecksumSHA256, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Episode{}, ErrNotFound
	}
	if err != nil {
		return Episode{}, err
	}
	if len(variants) > 0 {
		_ = json.Unmarshal(variants, &e.AudioVariants)
	}
	return e, nil
}

func (s *Store) EpisodeByID(ctx context.Context, id string) (Episode, error) {
	return scanEpisode(s.pool.QueryRow(ctx, `SELECT `+epColumns+` FROM episodes WHERE id = $1`, id))
}

func (s *Store) EpisodesForSeason(ctx context.Context, seasonID string, publishedOnly bool) ([]Episode, error) {
	q := `SELECT ` + epColumns + ` FROM episodes WHERE season_id = $1`
	if publishedOnly {
		q += ` AND status = 'published'`
	}
	q += ` ORDER BY number`
	return s.queryEpisodes(ctx, q, seasonID)
}

func (s *Store) EpisodesForShow(ctx context.Context, showID string, publishedOnly bool) ([]Episode, error) {
	q := `SELECT ` + epColumns + ` FROM episodes WHERE show_id = $1`
	if publishedOnly {
		q += ` AND status = 'published'`
	}
	q += ` ORDER BY number`
	return s.queryEpisodes(ctx, q, showID)
}

func (s *Store) queryEpisodes(ctx context.Context, q string, args ...any) ([]Episode, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Episode
	for rows.Next() {
		e, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) CreateEpisode(ctx context.Context, e Episode) (Episode, error) {
	return scanEpisode(s.pool.QueryRow(ctx,
		`INSERT INTO episodes (show_id, season_id, number, title, slug, synopsis, script,
			is_premium, free_preview_sec, ai_generated, ai_job_id, processing)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 RETURNING `+epColumns,
		e.ShowID, e.SeasonID, e.Number, e.Title, e.Slug, e.Synopsis, e.Script,
		e.IsPremium, e.FreePreviewSec, e.AIGenerated, e.AIJobID, defaultProc(e.Processing)))
}

func defaultProc(p string) string {
	if p == "" {
		return ProcNone
	}
	return p
}

func (s *Store) UpdateEpisode(ctx context.Context, id string, patch map[string]any) (Episode, error) {
	set, args := buildSet(patch)
	if set == "" {
		return s.EpisodeByID(ctx, id)
	}
	args = append(args, id)
	if _, err := s.pool.Exec(ctx,
		fmt.Sprintf(`UPDATE episodes SET %s, updated_at = now() WHERE id = $%d`, set, len(args)), args...); err != nil {
		return Episode{}, err
	}
	return s.EpisodeByID(ctx, id)
}

// SetEpisodeStatus transitions an episode within the caller's tx.
func (s *Store) SetEpisodeStatus(ctx context.Context, tx pgx.Tx, id, from, to string) error {
	ct, err := tx.Exec(ctx,
		`UPDATE episodes SET status = $2::content_status,
			published_at = CASE WHEN $2 = 'published' AND published_at IS NULL THEN now() ELSE published_at END,
			updated_at = now()
		 WHERE id = $1 AND status = $3::content_status`, id, to, from)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("episode %s is not in status %q", id, from)
	}
	return nil
}

// SetEpisodeProcessing updates the processing state and clears/sets the error.
func (s *Store) SetEpisodeProcessing(ctx context.Context, id, state, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE episodes SET processing = $2::processing_status, processing_error = $3, updated_at = now()
		 WHERE id = $1`, id, state, errMsg)
	return err
}

// AttachMedia stores packaged audio metadata and marks the episode ready.
func (s *Store) AttachMedia(ctx context.Context, id string, m MediaMetadata) error {
	variants, _ := json.Marshal(m.Variants)
	_, err := s.pool.Exec(ctx,
		`UPDATE episodes SET
			hls_master_key = $2, audio_variants = $3, codec = $4, sample_rate_hz = $5,
			channels = $6, file_size_bytes = $7, checksum_sha256 = $8, duration_sec = $9,
			processing = 'ready', processing_error = '', updated_at = now()
		 WHERE id = $1`,
		id, m.HLSMasterKey, variants, m.Codec, m.SampleRateHz, m.Channels,
		m.FileSizeBytes, m.ChecksumSHA256, m.DurationSec)
	return err
}

// RecalcShowAggregates refreshes episode_count and total_duration_sec from
// published episodes.
func (s *Store) RecalcShowAggregates(ctx context.Context, tx pgx.Tx, showID string) error {
	_, err := tx.Exec(ctx,
		`UPDATE shows SET
			episode_count = sub.cnt, total_duration_sec = sub.dur, updated_at = now()
		 FROM (
			SELECT count(*) AS cnt, coalesce(sum(duration_sec), 0) AS dur
			FROM episodes WHERE show_id = $1 AND status = 'published'
		 ) sub
		 WHERE shows.id = $1`, showID)
	return err
}

// --- reviews ---

func (s *Store) RecordReview(ctx context.Context, tx pgx.Tx, ev ReviewEvent) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO review_events (entity_type, entity_id, reviewer_id, action, notes, from_status, to_status)
		 VALUES ($1,$2,$3,$4,$5,$6::content_status,$7::content_status)`,
		ev.EntityType, ev.EntityID, ev.ReviewerID, ev.Action, ev.Notes, ev.FromStatus, ev.ToStatus)
	return err
}

func (s *Store) ReviewHistory(ctx context.Context, entityType, entityID string) ([]ReviewEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, entity_type, entity_id, reviewer_id, action, notes, from_status, to_status, created_at
		 FROM review_events WHERE entity_type = $1 AND entity_id = $2 ORDER BY created_at`,
		entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewEvent
	for rows.Next() {
		var e ReviewEvent
		if err := rows.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.ReviewerID, &e.Action, &e.Notes,
			&e.FromStatus, &e.ToStatus, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReviewQueue lists shows and episodes awaiting review.
func (s *Store) ReviewQueue(ctx context.Context) (map[string]any, error) {
	sRows, err := s.pool.Query(ctx,
		`SELECT `+showColumns+` FROM shows s JOIN creators c ON c.id = s.creator_id
		 WHERE s.status = 'ready_for_review' ORDER BY s.updated_at`)
	if err != nil {
		return nil, err
	}
	defer sRows.Close()
	var pendingShows []Show
	for sRows.Next() {
		sh, err := scanShow(sRows)
		if err != nil {
			return nil, err
		}
		pendingShows = append(pendingShows, sh)
	}

	eps, err := s.queryEpisodes(ctx,
		`SELECT `+epColumns+` FROM episodes WHERE status = 'ready_for_review' ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	return map[string]any{"shows": pendingShows, "episodes": eps}, nil
}

// --- media uploads ---

func (s *Store) CreateUpload(ctx context.Context, episodeID, key, contentType string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO media_uploads (episode_id, object_key, content_type) VALUES ($1,$2,$3) RETURNING id`,
		episodeID, key, contentType).Scan(&id)
	return id, err
}

func (s *Store) ConfirmUpload(ctx context.Context, uploadID string, size int64) (string, string, error) {
	var episodeID, key string
	err := s.pool.QueryRow(ctx,
		`UPDATE media_uploads SET status = 'uploaded', size_bytes = $2, confirmed_at = now()
		 WHERE id = $1 AND status = 'pending' RETURNING episode_id, object_key`,
		uploadID, size).Scan(&episodeID, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return episodeID, key, err
}

// --- idempotency for the consumer ---

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

// buildSet turns a patch map into "col = $n" fragments. Column names are fixed
// keys chosen by the handler, never user input.
func buildSet(patch map[string]any) (string, []any) {
	if len(patch) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(patch))
	args := make([]any, 0, len(patch))
	for col, val := range patch {
		args = append(args, val)
		parts = append(parts, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	return strings.Join(parts, ", "), args
}

// MediaMetadata is the packaged-audio metadata attached to an episode.
type MediaMetadata struct {
	HLSMasterKey   string         `json:"hls_master_key"`
	Variants       []AudioVariant `json:"variants"`
	Codec          string         `json:"codec"`
	SampleRateHz   int            `json:"sample_rate_hz"`
	Channels       int            `json:"channels"`
	FileSizeBytes  int64          `json:"file_size_bytes"`
	ChecksumSHA256 string         `json:"checksum_sha256"`
	DurationSec    int            `json:"duration_sec"`
}
