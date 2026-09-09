package internal

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/kafkax"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/objectstore"
	"github.com/auralis/platform/outbox"
	"github.com/auralis/platform/telemetry"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const service = "content"

// App holds the content service dependencies.
type App struct {
	Store        *Store
	Objects      *objectstore.Client
	ServiceToken string
	UploadTTL    time.Duration
	MaxUploadMB  int64
}

// NewApp builds an App.
func NewApp(store *Store, objects *objectstore.Client, serviceToken string) *App {
	return &App{
		Store: store, Objects: objects, ServiceToken: serviceToken,
		UploadTTL: 30 * time.Minute, MaxUploadMB: 500,
	}
}

// Routes registers the content endpoints.
func (a *App) Routes(r chi.Router) {
	// Public catalog and search.
	r.Get("/genres", a.listGenres)
	r.Get("/languages", a.listLanguages)
	r.Get("/shows", a.listShows)
	r.Get("/shows/{slug}", a.getShow)
	r.Get("/shows/{id}/seasons", a.getShowSeasons)
	r.Get("/shows/{id}/episodes", a.getShowEpisodes)
	r.Get("/seasons/{id}/episodes", a.getSeasonEpisodes)
	r.Get("/episodes/{id}", a.getEpisode)
	r.Get("/search", a.search)

	// Internal, service-to-service: episode detail regardless of publish state,
	// used by playback for authorization and by recommendation for features.
	r.Get("/internal/episodes/{id}", a.getEpisodeInternal)
	r.Get("/internal/shows/{id}", a.getShowInternal)

	// Creator authoring.
	r.Group(func(r chi.Router) {
		r.Use(authn.RequireRoles(authn.RoleCreator, authn.RoleAdmin))
		r.Post("/shows", a.createShow)
		r.Get("/creator/shows", a.listMyShows)
		r.Get("/creator/shows/{id}/episodes", a.listMyShowEpisodes)
		r.Patch("/shows/{id}", a.updateShow)
		r.Post("/shows/{id}/seasons", a.createSeason)
		r.Post("/seasons/{id}/episodes", a.createEpisode)
		r.Patch("/episodes/{id}", a.updateEpisode)
		r.Post("/episodes/{id}/script", a.editScript)
		r.Post("/episodes/{id}/upload-url", a.createUploadURL)
		r.Post("/episodes/{id}/media/confirm", a.confirmUpload)
		r.Post("/shows/{id}/submit", a.submitShow)
		r.Post("/episodes/{id}/submit", a.submitEpisode)
		r.Get("/shows/{id}/review-history", a.showReviewHistory)
	})

	// Admin / reviewer.
	r.Group(func(r chi.Router) {
		r.Use(authn.RequireRoles(authn.RoleAdmin))
		r.Get("/admin/review-queue", a.reviewQueue)
		r.Post("/admin/review/{type}/{id}", a.review)
	})

	// Service-to-service authoring, used by the ai-media pipeline.
	r.Group(func(r chi.Router) {
		r.Use(authn.ServiceToken(a.ServiceToken))
		r.Post("/internal/authoring/shows", a.internalCreateShow)
		r.Post("/internal/authoring/episodes", a.internalCreateEpisode)
		r.Patch("/internal/authoring/shows/{id}", a.internalUpdateShow)
		r.Patch("/internal/authoring/episodes/{id}", a.internalUpdateEpisode)
	})
}

// --- shared helpers ---

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func (a *App) enqueue(ctx context.Context, tx pgx.Tx, eventType string, version int, key string, payload any) error {
	env, err := envelope.New(eventType, version, service, logging.FromContext(ctx).CorrelationID, "", payload)
	if err != nil {
		return err
	}
	return outbox.Enqueue(ctx, tx, kafkax.TopicContentEvents, key, env)
}

func creatorNameFrom(id authn.Identity, fallback string) string {
	if fallback != "" {
		return fallback
	}
	return "Creator " + shortID(id.UserID)
}

func shortID(s string) string {
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}

// ownsShow verifies the caller is the show's creator, or an admin.
func (a *App) ownsShow(ctx context.Context, id authn.Identity, showID string) (Show, error) {
	sh, err := a.Store.ShowByID(ctx, showID)
	if err != nil {
		return Show{}, errcodes.Missing("show not found")
	}
	if id.HasRole(authn.RoleAdmin) {
		return sh, nil
	}
	creator, err := a.Store.CreatorByUser(ctx, id.UserID)
	if err != nil || creator.ID != sh.CreatorID {
		return Show{}, errcodes.Denied("you do not own this show")
	}
	return sh, nil
}

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func boolPtrParam(r *http.Request, key string) *bool {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	b := v == "true" || v == "1"
	return &b
}

func metric(name, outcome string) { telemetry.Count(service, name, outcome) }
