package internal

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/httpx"
	"github.com/auralis/platform/kafkax"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/outbox"
	"github.com/auralis/platform/svcclient"
	"github.com/auralis/platform/telemetry"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const service = "playback"

// valid client event types.
var eventTypes = map[string]bool{
	"PLAY": true, "PAUSE": true, "SEEK": true, "PROGRESS": true, "COMPLETE": true,
	"SKIP": true, "BUFFER_START": true, "BUFFER_END": true,
}

// domainEventType maps a client event type to its canonical Kafka event type.
// COMPLETE becomes "playback.completed" to match the type emitted by the
// progress endpoint and expected by every downstream consumer; the rest are the
// lowercased client type (PLAY -> playback.play, BUFFER_START -> playback.buffer_start).
func domainEventType(clientType string) string {
	if clientType == "COMPLETE" {
		return "playback.completed"
	}
	return "playback." + strings.ToLower(clientType)
}

// MediaSigner produces download URLs for object keys. objectstore.Client
// satisfies it; tests provide a fake.
//
// PublicURL returns a stable, unauthenticated URL when the bucket is fronted by
// a public base URL (an R2 public domain or a CDN), and "" otherwise. HLS is
// served this way in production: the player resolves the child playlists and
// every .ts segment relative to the master URL with no query string, so a
// presigned master alone leaves those requests unsigned. A public base URL
// makes the whole tree reachable; presigned URLs are the local-MinIO fallback.
type MediaSigner interface {
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	PublicURL(key string) string
}

// App holds the playback service dependencies.
type App struct {
	Store     *Store
	Objects   MediaSigner
	Content   *svcclient.Client
	Users     *svcclient.Client
	SignedTTL time.Duration
}

func NewApp(store *Store, objects MediaSigner, content, users *svcclient.Client) *App {
	return &App{Store: store, Objects: objects, Content: content, Users: users, SignedTTL: 2 * time.Hour}
}

// mediaURL returns a public URL for key when the bucket has a public base URL
// configured, and a presigned URL otherwise.
func (a *App) mediaURL(ctx context.Context, key string) (string, error) {
	if u := a.Objects.PublicURL(key); u != "" {
		return u, nil
	}
	return a.Objects.PresignGet(ctx, key, a.SignedTTL)
}

// Routes registers playback endpoints. All require an authenticated caller.
func (a *App) Routes(r chi.Router) {
	r.Route("/playback", func(r chi.Router) {
		r.Use(authn.RequireRoles())
		r.Post("/authorize", a.authorize)
		r.Get("/progress/{episodeId}", a.getProgress)
		r.Post("/progress", a.postProgress)
		r.Post("/events", a.postEvents)
		r.Get("/continue", a.continueListening)
		r.Get("/devices", a.listDevices)
		r.Put("/devices", a.upsertDevice)
	})
}

// --- authorization ---

type authorizeReq struct {
	EpisodeID string `json:"episode_id"`
	DeviceID  string `json:"device_id"`
}

func (a *App) authorize(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	var req authorizeReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.EpisodeID == "" {
		httpx.Error(w, r, errcodes.BadRequest("episode_id is required"))
		return
	}
	ctx := r.Context()

	meta, err := a.episodeMeta(ctx, req.EpisodeID)
	if err != nil {
		telemetry.Count(service, "authorize", "not_found")
		httpx.Error(w, r, errcodes.Missing("episode is not available for playback"))
		return
	}
	if !meta.Published || meta.HLSMasterKey == "" {
		telemetry.Count(service, "authorize", "not_published")
		httpx.Error(w, r, errcodes.Missing("episode is not available for playback"))
		return
	}

	previewOnly := false
	if meta.IsPremium {
		premium, err := a.hasPremium(ctx, id.UserID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		if !premium {
			if meta.FreePreviewSec <= 0 {
				telemetry.Count(service, "authorize", "denied_premium")
				httpx.Error(w, r, errcodes.New(http.StatusPaymentRequired, "playback.premium_required",
					"this episode is part of Auralis Premium"))
				return
			}
			previewOnly = true
		}
	}

	master, err := a.mediaURL(ctx, meta.HLSMasterKey)
	if err != nil {
		httpx.Error(w, r, errcodes.New(http.StatusBadGateway, errcodes.Unavailable, "media storage unavailable"))
		return
	}
	variants := make([]map[string]any, 0, len(meta.AudioVariants))
	for _, v := range meta.AudioVariants {
		signed, err := a.mediaURL(ctx, v.Key)
		if err != nil {
			continue
		}
		variants = append(variants, map[string]any{
			"bitrate_kbps": v.BitrateKbps, "codec": v.Codec, "url": signed,
		})
	}

	var deviceID *string
	if did, derr := a.Store.UpsertDevice(ctx, id.UserID, req.DeviceID, deviceName(r), deviceKind(r)); derr == nil {
		deviceID = &did
	}
	sessionID, _ := a.Store.OpenSession(ctx, id.UserID, meta.EpisodeID, meta.ShowID, deviceID)

	prog, _ := a.Store.Progress(ctx, id.UserID, meta.EpisodeID)

	telemetry.Count(service, "authorize", "granted")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"session_id":          sessionID,
		"device_id":           deviceID,
		"episode":             meta,
		"hls_master_url":      master,
		"variants":            variants,
		"resume_position_sec": prog.PositionSec,
		"preview_only":        previewOnly,
		"preview_limit_sec":   previewLimit(meta, previewOnly),
		"expires_at":          time.Now().Add(a.SignedTTL).UTC(),
	})
}

func previewLimit(meta EpisodeMeta, previewOnly bool) int {
	if previewOnly {
		return meta.FreePreviewSec
	}
	return 0
}

// episodeMeta reads the local cache, falling back to a content service lookup
// (which also repopulates the cache) on a miss.
func (a *App) episodeMeta(ctx context.Context, episodeID string) (EpisodeMeta, error) {
	if m, err := a.Store.EpisodeMeta(ctx, episodeID); err == nil {
		return m, nil
	}
	return a.refreshEpisode(ctx, episodeID)
}

func (a *App) hasPremium(ctx context.Context, userID string) (bool, error) {
	var resp struct {
		PremiumActive bool `json:"premium_active"`
	}
	if err := a.Users.Get(ctx, "/internal/users/"+userID+"/entitlement", &resp); err != nil {
		telemetry.Count(service, "entitlement_check", "error")
		return false, errcodes.New(http.StatusBadGateway, errcodes.Unavailable,
			"could not verify your subscription, please try again")
	}
	return resp.PremiumActive, nil
}

// --- progress ---

type progressReq struct {
	EpisodeID     string `json:"episode_id"`
	ShowID        string `json:"show_id"`
	SessionID     string `json:"session_id"`
	PositionSec   int    `json:"position_sec"`
	DurationSec   int    `json:"duration_sec"`
	Completed     bool   `json:"completed"`
	ClientEventID string `json:"client_event_id"`
	OccurredAt    string `json:"occurred_at"`
}

func (a *App) postProgress(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	var req progressReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.EpisodeID == "" || req.PositionSec < 0 {
		httpx.Error(w, r, errcodes.BadRequest("episode_id and a non-negative position_sec are required"))
		return
	}
	occurredAt := parseTime(req.OccurredAt)
	ctx := r.Context()

	showID, durationSec := req.ShowID, req.DurationSec
	if m, err := a.Store.EpisodeMeta(ctx, req.EpisodeID); err == nil {
		if showID == "" {
			showID = m.ShowID
		}
		if durationSec <= 0 {
			durationSec = m.DurationSec
		}
	}

	tx, err := a.Store.Pool().Begin(ctx)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("database unavailable"))
		return
	}
	defer tx.Rollback(ctx)

	if req.ClientEventID != "" {
		fresh, err := a.Store.MarkSeen(ctx, tx, req.ClientEventID, id.UserID)
		if err != nil {
			httpx.Error(w, r, errcodes.Unexpected("could not record event"))
			return
		}
		if !fresh {
			// Duplicate: return the current stored progress without side effects.
			_ = tx.Rollback(ctx)
			p, _ := a.Store.Progress(ctx, id.UserID, req.EpisodeID)
			telemetry.Count(service, "progress", "duplicate")
			httpx.JSON(w, http.StatusOK, p)
			return
		}
	}

	prog, err := a.Store.SaveProgress(ctx, tx, id.UserID, req.EpisodeID, showID,
		req.PositionSec, durationSec, req.Completed, occurredAt)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not save progress"))
		return
	}

	eventType := "playback.progress"
	if prog.Completed {
		eventType = "playback.completed"
	}
	if err := a.enqueue(ctx, tx, eventType, id.UserID, map[string]any{
		"user_id": id.UserID, "episode_id": req.EpisodeID, "show_id": prog.ShowID,
		"position_sec": prog.PositionSec, "duration_sec": prog.DurationSec,
		"completed": prog.Completed, "session_id": req.SessionID, "occurred_at": occurredAt,
	}); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not record playback event"))
		return
	}
	if req.SessionID != "" {
		_ = a.Store.TouchSession(ctx, req.SessionID, id.UserID)
	}
	if err := tx.Commit(ctx); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not save progress"))
		return
	}
	telemetry.Count(service, "progress", "saved")
	httpx.JSON(w, http.StatusOK, prog)
}

func (a *App) getProgress(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	p, err := a.Store.Progress(r.Context(), id.UserID, chi.URLParam(r, "episodeId"))
	if err != nil && err != ErrNotFound {
		httpx.Error(w, r, errcodes.Unexpected("could not load progress"))
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// --- batched events ---

type eventItem struct {
	Type          string `json:"type"`
	EpisodeID     string `json:"episode_id"`
	ShowID        string `json:"show_id"`
	SessionID     string `json:"session_id"`
	PositionSec   int    `json:"position_sec"`
	FromSec       *int   `json:"from_sec"`
	ToSec         *int   `json:"to_sec"`
	ClientEventID string `json:"client_event_id"`
	OccurredAt    string `json:"occurred_at"`
}

// postEvents accepts a batch of playback events. PLAY/PAUSE/SEEK/SKIP/BUFFER_*
// are forwarded to Kafka for analytics; PROGRESS and COMPLETE also update the
// stored resume position. Duplicate client_event_ids are ignored.
func (a *App) postEvents(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	var body struct {
		Events []eventItem `json:"events"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if len(body.Events) == 0 || len(body.Events) > 200 {
		httpx.Error(w, r, errcodes.BadRequest("events must contain between 1 and 200 items"))
		return
	}
	ctx := r.Context()
	accepted, skipped := 0, 0

	for _, ev := range body.Events {
		if !eventTypes[ev.Type] || ev.EpisodeID == "" {
			skipped++
			continue
		}
		// Enrich from the authoritative episode cache: the client may omit
		// show_id and never knows the real duration, which analytics needs to
		// credit listening time on completion.
		showID, durationSec := ev.ShowID, 0
		if meta, merr := a.Store.EpisodeMeta(ctx, ev.EpisodeID); merr == nil {
			if showID == "" {
				showID = meta.ShowID
			}
			durationSec = meta.DurationSec
		}
		tx, err := a.Store.Pool().Begin(ctx)
		if err != nil {
			httpx.Error(w, r, errcodes.Unexpected("database unavailable"))
			return
		}
		if ev.ClientEventID != "" {
			fresh, err := a.Store.MarkSeen(ctx, tx, ev.ClientEventID, id.UserID)
			if err != nil {
				_ = tx.Rollback(ctx)
				httpx.Error(w, r, errcodes.Unexpected("could not record events"))
				return
			}
			if !fresh {
				_ = tx.Rollback(ctx)
				skipped++
				continue
			}
		}
		occurredAt := parseTime(ev.OccurredAt)
		if ev.Type == "PROGRESS" || ev.Type == "COMPLETE" {
			if _, err := a.Store.SaveProgress(ctx, tx, id.UserID, ev.EpisodeID, showID,
				ev.PositionSec, durationSec, ev.Type == "COMPLETE", occurredAt); err != nil {
				_ = tx.Rollback(ctx)
				httpx.Error(w, r, errcodes.Unexpected("could not save progress"))
				return
			}
		}
		payload := map[string]any{
			"user_id": id.UserID, "episode_id": ev.EpisodeID, "show_id": showID,
			"event_type": ev.Type, "position_sec": ev.PositionSec, "duration_sec": durationSec,
			"completed": ev.Type == "COMPLETE", "session_id": ev.SessionID,
			"occurred_at": occurredAt,
		}
		if ev.FromSec != nil {
			payload["from_sec"] = *ev.FromSec
		}
		if ev.ToSec != nil {
			payload["to_sec"] = *ev.ToSec
		}
		if err := a.enqueue(ctx, tx, domainEventType(ev.Type), id.UserID, payload); err != nil {
			_ = tx.Rollback(ctx)
			httpx.Error(w, r, errcodes.Unexpected("could not record events"))
			return
		}
		if err := tx.Commit(ctx); err != nil {
			httpx.Error(w, r, errcodes.Unexpected("could not record events"))
			return
		}
		accepted++
	}
	telemetry.Count(service, "events_batch", "processed")
	httpx.JSON(w, http.StatusOK, map[string]any{"accepted": accepted, "skipped": skipped})
}

func (a *App) continueListening(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	limit := 20
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 50 {
		limit = v
	}
	items, err := a.Store.ContinueListening(r.Context(), id.UserID, limit)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load continue listening"))
		return
	}
	if items == nil {
		items = []Progress{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) listDevices(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	devices, err := a.Store.Devices(r.Context(), id.UserID)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not list devices"))
		return
	}
	if devices == nil {
		devices = []map[string]any{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (a *App) upsertDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	var req struct {
		DeviceID string `json:"device_id"`
		Name     string `json:"name"`
		Kind     string `json:"kind"`
	}
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.Kind == "" {
		req.Kind = "web"
	}
	if req.Name == "" {
		req.Name = "Auralis Web"
	}
	deviceID, err := a.Store.UpsertDevice(r.Context(), id.UserID, req.DeviceID, req.Name, req.Kind)
	if err != nil {
		httpx.Error(w, r, errcodes.BadRequest("could not register device"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"device_id": deviceID})
}

// --- helpers ---

func (a *App) enqueue(ctx context.Context, tx pgx.Tx, eventType, key string, payload any) error {
	env, err := envelope.New(eventType, 1, service, logging.FromContext(ctx).CorrelationID, "", payload)
	if err != nil {
		return err
	}
	return outbox.Enqueue(ctx, tx, kafkax.TopicPlaybackEvents, key, env)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Now().UTC()
}

func deviceName(r *http.Request) string {
	if ua := r.UserAgent(); ua != "" {
		if len(ua) > 60 {
			return ua[:60]
		}
		return ua
	}
	return "Auralis Web"
}

func deviceKind(r *http.Request) string {
	ua := strings.ToLower(r.UserAgent())
	switch {
	case strings.Contains(ua, "mobile"):
		return "mobile"
	case strings.Contains(ua, "tablet") || strings.Contains(ua, "ipad"):
		return "tablet"
	default:
		return "web"
	}
}
