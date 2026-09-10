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
	"github.com/auralis/platform/telemetry"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const service = "user"

// App holds the user service dependencies.
type App struct {
	Store        *Store
	ServiceToken string
}

func NewApp(store *Store, serviceToken string) *App {
	return &App{Store: store, ServiceToken: serviceToken}
}

// Routes registers the user endpoints. Everything under /me requires an
// authenticated caller; the service reads identity from gateway headers.
func (a *App) Routes(r chi.Router) {
	r.Route("/me", func(r chi.Router) {
		r.Use(authn.RequireRoles()) // any authenticated user
		r.Get("/profile", a.getProfile)
		r.Put("/profile", a.updateProfile)
		r.Get("/preferences", a.getPreferences)
		r.Put("/preferences", a.updatePreferences)

		r.Get("/likes", a.listLikes)
		r.Post("/likes", a.addLike)
		r.Delete("/likes/{type}/{id}", a.removeLike)

		r.Get("/bookmarks", a.listBookmarks)
		r.Post("/bookmarks", a.addBookmark)
		r.Delete("/bookmarks/{episodeId}", a.removeBookmark)

		r.Get("/follows", a.listFollows)
		r.Post("/follows/{showId}", a.addFollow)
		r.Delete("/follows/{showId}", a.removeFollow)

		r.Get("/history", a.listHistory)

		r.Get("/entitlement", a.getEntitlement)
		r.Post("/entitlement/redeem", a.redeemPromo)
	})

	// Service-to-service: playback checks premium entitlement before authorizing
	// a premium stream. Recommendation reads preferences to seed cold-start.
	r.Group(func(r chi.Router) {
		r.Use(authn.ServiceToken(a.ServiceToken))
		r.Get("/internal/users/{id}/entitlement", a.internalEntitlement)
		r.Get("/internal/users/{id}/preferences", a.internalPreferences)
	})

	// Admin: grant or revoke entitlements directly.
	r.Group(func(r chi.Router) {
		r.Use(authn.RequireRoles(authn.RoleAdmin))
		r.Post("/admin/users/{id}/entitlement", a.adminSetEntitlement)
	})
}

// --- profile / preferences ---

func (a *App) getProfile(w http.ResponseWriter, r *http.Request) {
	id := caller(r)
	p, err := a.Store.Profile(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("profile not found"))
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (a *App) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"display_name"`
		AvatarURL   string `json:"avatar_url"`
		Bio         string `json:"bio"`
	}
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if len([]rune(req.DisplayName)) < 1 || len([]rune(req.DisplayName)) > 60 {
		httpx.Error(w, r, errcodes.BadRequest("display_name must be 1 to 60 characters"))
		return
	}
	if len([]rune(req.Bio)) > 400 {
		httpx.Error(w, r, errcodes.BadRequest("bio must be 400 characters or fewer"))
		return
	}
	p, err := a.Store.UpdateProfile(r.Context(), caller(r), req.DisplayName, req.AvatarURL, req.Bio)
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("profile not found"))
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (a *App) getPreferences(w http.ResponseWriter, r *http.Request) {
	p, err := a.Store.Preferences(r.Context(), caller(r))
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("preferences not found"))
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (a *App) updatePreferences(w http.ResponseWriter, r *http.Request) {
	var req Preferences
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	req.UserID = caller(r)
	if req.PlaybackSpeed == 0 {
		req.PlaybackSpeed = 1.0
	}
	if req.PlaybackSpeed < 0.5 || req.PlaybackSpeed > 3.0 {
		httpx.Error(w, r, errcodes.BadRequest("playback_speed must be between 0.5 and 3.0"))
		return
	}
	req.GenreSlugs = trimList(req.GenreSlugs, 20)
	req.LanguageCodes = trimList(req.LanguageCodes, 10)

	var updated Preferences
	err := a.mutate(r.Context(), "user.preferences_changed", req.UserID,
		func(ctx context.Context, tx pgx.Tx) (bool, map[string]any, error) {
			var err error
			updated, err = a.Store.UpdatePreferences(ctx, tx, req)
			if err != nil {
				return false, nil, err
			}
			return true, map[string]any{
				"user_id": updated.UserID, "genre_slugs": updated.GenreSlugs,
				"language_codes": updated.LanguageCodes, "explicit_ok": updated.ExplicitOK,
			}, nil
		})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not update preferences"))
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

// --- likes ---

type likeReq struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	ShowID string `json:"show_id"`
}

func (a *App) addLike(w http.ResponseWriter, r *http.Request) {
	var req likeReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.Type != "show" && req.Type != "episode" {
		httpx.Error(w, r, errcodes.BadRequest("type must be show or episode"))
		return
	}
	if req.ID == "" || req.ShowID == "" {
		httpx.Error(w, r, errcodes.BadRequest("id and show_id are required"))
		return
	}
	uid := caller(r)
	err := a.mutate(r.Context(), "user.liked", uid,
		func(ctx context.Context, tx pgx.Tx) (bool, map[string]any, error) {
			added, err := a.Store.AddLike(ctx, tx, uid, req.Type, req.ID, req.ShowID)
			return added, map[string]any{
				"user_id": uid, "target_type": req.Type, "target_id": req.ID, "show_id": req.ShowID,
			}, err
		})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not add like"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"liked": true})
}

func (a *App) removeLike(w http.ResponseWriter, r *http.Request) {
	t, targetID := chi.URLParam(r, "type"), chi.URLParam(r, "id")
	uid := caller(r)
	err := a.mutate(r.Context(), "user.unliked", uid,
		func(ctx context.Context, tx pgx.Tx) (bool, map[string]any, error) {
			removed, err := a.Store.RemoveLike(ctx, tx, uid, t, targetID)
			return removed, map[string]any{
				"user_id": uid, "target_type": t, "target_id": targetID,
			}, err
		})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not remove like"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"liked": false})
}

func (a *App) listLikes(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	likes, err := a.Store.Likes(r.Context(), caller(r), limit, offset)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not list likes"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"likes": nonNilLikes(likes)})
}

// --- bookmarks ---

func (a *App) addBookmark(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EpisodeID string `json:"episode_id"`
		ShowID    string `json:"show_id"`
		Note      string `json:"note"`
	}
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.EpisodeID == "" || req.ShowID == "" {
		httpx.Error(w, r, errcodes.BadRequest("episode_id and show_id are required"))
		return
	}
	if len([]rune(req.Note)) > 280 {
		httpx.Error(w, r, errcodes.BadRequest("note must be 280 characters or fewer"))
		return
	}
	uid := caller(r)
	err := a.mutate(r.Context(), "user.bookmarked", uid,
		func(ctx context.Context, tx pgx.Tx) (bool, map[string]any, error) {
			added, err := a.Store.AddBookmark(ctx, tx, uid, req.EpisodeID, req.ShowID, req.Note)
			return added, map[string]any{
				"user_id": uid, "episode_id": req.EpisodeID, "show_id": req.ShowID,
			}, err
		})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not add bookmark"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"bookmarked": true})
}

func (a *App) removeBookmark(w http.ResponseWriter, r *http.Request) {
	episodeID := chi.URLParam(r, "episodeId")
	if _, err := a.Store.RemoveBookmark(r.Context(), caller(r), episodeID); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not remove bookmark"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"bookmarked": false})
}

func (a *App) listBookmarks(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	b, err := a.Store.Bookmarks(r.Context(), caller(r), limit, offset)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not list bookmarks"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"bookmarks": nonNilBookmarks(b)})
}

// --- follows ---

func (a *App) addFollow(w http.ResponseWriter, r *http.Request) {
	showID := chi.URLParam(r, "showId")
	uid := caller(r)
	err := a.mutate(r.Context(), "user.followed", uid,
		func(ctx context.Context, tx pgx.Tx) (bool, map[string]any, error) {
			added, err := a.Store.AddFollow(ctx, tx, uid, showID)
			return added, map[string]any{"user_id": uid, "show_id": showID}, err
		})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not follow show"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"following": true})
}

func (a *App) removeFollow(w http.ResponseWriter, r *http.Request) {
	showID := chi.URLParam(r, "showId")
	uid := caller(r)
	err := a.mutate(r.Context(), "user.unfollowed", uid,
		func(ctx context.Context, tx pgx.Tx) (bool, map[string]any, error) {
			removed, err := a.Store.RemoveFollow(ctx, tx, uid, showID)
			return removed, map[string]any{"user_id": uid, "show_id": showID}, err
		})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not unfollow show"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"following": false})
}

func (a *App) listFollows(w http.ResponseWriter, r *http.Request) {
	f, err := a.Store.Follows(r.Context(), caller(r))
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not list follows"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"follows": nonNilFollows(f)})
}

// --- history ---

func (a *App) listHistory(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	h, err := a.Store.History(r.Context(), caller(r), limit, offset)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not list history"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"history": nonNilHistory(h)})
}

// --- entitlements ---

func (a *App) getEntitlement(w http.ResponseWriter, r *http.Request) {
	e, err := a.Store.Entitlement(r.Context(), caller(r))
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load entitlement"))
		return
	}
	httpx.JSON(w, http.StatusOK, entitlementView(e))
}

func (a *App) redeemPromo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		httpx.Error(w, r, errcodes.BadRequest("code is required"))
		return
	}
	uid := caller(r)
	var ent Entitlement
	err := a.mutate(r.Context(), "user.entitlement_changed", uid,
		func(ctx context.Context, tx pgx.Tx) (bool, map[string]any, error) {
			var err error
			ent, err = a.Store.RedeemPromo(ctx, tx, uid, code)
			if err != nil {
				return false, nil, err
			}
			return true, map[string]any{
				"user_id": ent.UserID, "plan": ent.Plan, "source": ent.Source, "expires_at": ent.ExpiresAt,
			}, nil
		})
	if err != nil {
		if err == ErrNotFound {
			httpx.Error(w, r, errcodes.Missing("unknown promo code"))
			return
		}
		httpx.Error(w, r, errcodes.BadRequest(err.Error()))
		return
	}
	telemetry.Count(service, "promo_redeem", "success")
	httpx.JSON(w, http.StatusOK, entitlementView(ent))
}

func (a *App) adminSetEntitlement(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var req struct {
		Plan         string `json:"plan"`
		DurationDays int    `json:"duration_days"`
	}
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.Plan != "free" && req.Plan != "premium" {
		httpx.Error(w, r, errcodes.BadRequest("plan must be free or premium"))
		return
	}
	var expires *time.Time
	if req.Plan == "premium" {
		days := req.DurationDays
		if days <= 0 {
			days = 365
		}
		t := time.Now().AddDate(0, 0, days)
		expires = &t
	}
	ctx := r.Context()
	tx, err := a.Store.Pool().Begin(ctx)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("database unavailable"))
		return
	}
	defer tx.Rollback(ctx)
	ent, err := a.Store.SetEntitlement(ctx, tx, userID, req.Plan, "admin_grant", expires)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not set entitlement"))
		return
	}
	env, _ := envelope.New("user.entitlement_changed", 1, service, logging.FromContext(ctx).CorrelationID, "", map[string]any{
		"user_id": userID, "plan": ent.Plan, "source": "admin_grant", "expires_at": ent.ExpiresAt,
	})
	if err := outbox.Enqueue(ctx, tx, kafkax.TopicUserEvents, userID, env); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not record entitlement event"))
		return
	}
	if err := tx.Commit(ctx); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not save entitlement"))
		return
	}
	httpx.JSON(w, http.StatusOK, entitlementView(ent))
}

func (a *App) internalEntitlement(w http.ResponseWriter, r *http.Request) {
	e, err := a.Store.Entitlement(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load entitlement"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"user_id": e.UserID, "plan": e.Plan, "status": e.Status, "premium_active": e.Active(),
	})
}

func (a *App) internalPreferences(w http.ResponseWriter, r *http.Request) {
	p, err := a.Store.Preferences(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.JSON(w, http.StatusOK, Preferences{UserID: chi.URLParam(r, "id"), GenreSlugs: []string{}, LanguageCodes: []string{}, PlaybackSpeed: 1})
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// --- helpers ---

func caller(r *http.Request) string {
	id, _ := authn.FromContext(r.Context())
	return id.UserID
}

// mutate runs a library change and its domain event in one transaction: fn
// performs the write and reports whether anything actually changed plus the
// event payload. The event is enqueued to the outbox in the same tx, so the
// row change and the event commit together or not at all. Emitting the event
// from a second transaction (as this used to) drops it forever if that tx
// fails or the process dies in between, and recommendation/analytics then
// permanently miss the change.
func (a *App) mutate(ctx context.Context, eventType, key string,
	fn func(context.Context, pgx.Tx) (bool, map[string]any, error)) error {
	tx, err := a.Store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	changed, payload, err := fn(ctx, tx)
	if err != nil {
		return err
	}
	if changed {
		env, err := envelope.New(eventType, 1, service, logging.FromContext(ctx).CorrelationID, "", payload)
		if err != nil {
			return err
		}
		if err := outbox.Enqueue(ctx, tx, kafkax.TopicUserEvents, key, env); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func entitlementView(e Entitlement) map[string]any {
	return map[string]any{
		"user_id": e.UserID, "plan": e.Plan, "status": e.Status, "source": e.Source,
		"premium_active": e.Active(), "granted_at": e.GrantedAt, "expires_at": e.ExpiresAt,
	}
}

func page(r *http.Request) (limit, offset int) {
	limit, offset = 50, 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	return
}

func trimList(in []string, max int) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && !seen[s] && len(out) < max {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func nonNilLikes(in []Like) []Like {
	if in == nil {
		return []Like{}
	}
	return in
}
func nonNilBookmarks(in []Bookmark) []Bookmark {
	if in == nil {
		return []Bookmark{}
	}
	return in
}
func nonNilFollows(in []Follow) []Follow {
	if in == nil {
		return []Follow{}
	}
	return in
}
func nonNilHistory(in []HistoryEntry) []HistoryEntry {
	if in == nil {
		return []HistoryEntry{}
	}
	return in
}
