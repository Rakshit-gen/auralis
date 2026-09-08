package internal

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/httpx"
	"github.com/go-chi/chi/v5"
)

// submitShow moves a draft or rejected show to ready_for_review. The show must
// have at least one published-ready episode.
func (a *App) submitShow(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	sh, err := a.ownsShow(r.Context(), id, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if sh.Status != StatusDraft && sh.Status != StatusRejected {
		httpx.Error(w, r, errcodes.Conflicting("show is already "+sh.Status))
		return
	}
	eps, _ := a.Store.EpisodesForShow(r.Context(), sh.ID, false)
	ready := 0
	for _, e := range eps {
		if e.Processing == ProcReady || e.DurationSec > 0 {
			ready++
		}
	}
	if ready == 0 {
		httpx.Error(w, r, errcodes.BadRequest("a show needs at least one episode with processed audio before review"))
		return
	}
	a.transition(w, r, "show", sh.ID, sh.Status, StatusReadyForReview, "submit", "", id.UserID)
}

func (a *App) submitEpisode(w http.ResponseWriter, r *http.Request) {
	e, err := a.episodeOwned(w, r)
	if err != nil {
		return
	}
	if e.Status != StatusDraft && e.Status != StatusRejected {
		httpx.Error(w, r, errcodes.Conflicting("episode is already "+e.Status))
		return
	}
	if e.Processing != ProcReady && e.DurationSec == 0 {
		httpx.Error(w, r, errcodes.BadRequest("episode audio must finish processing before review"))
		return
	}
	id, _ := authn.FromContext(r.Context())
	a.transition(w, r, "episode", e.ID, e.Status, StatusReadyForReview, "submit", "", id.UserID)
}

func (a *App) reviewQueue(w http.ResponseWriter, r *http.Request) {
	q, err := a.Store.ReviewQueue(r.Context())
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load review queue"))
		return
	}
	httpx.JSON(w, http.StatusOK, q)
}

type reviewReq struct {
	Action string `json:"action"` // approve, reject, publish, archive
	Notes  string `json:"notes"`
}

// review applies a reviewer decision to a show or episode.
func (a *App) review(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	entityType := chi.URLParam(r, "type")
	entityID := chi.URLParam(r, "id")
	if entityType != "show" && entityType != "episode" {
		httpx.Error(w, r, errcodes.BadRequest("type must be show or episode"))
		return
	}
	var req reviewReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}

	from, err := a.currentStatus(r, entityType, entityID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	to, ok := reviewTransition(from, req.Action)
	if !ok {
		httpx.Error(w, r, errcodes.Conflicting(
			fmt.Sprintf("cannot %s a %s in status %q", req.Action, entityType, from)))
		return
	}
	if req.Action == "reject" && strings.TrimSpace(req.Notes) == "" {
		httpx.Error(w, r, errcodes.BadRequest("rejection requires notes"))
		return
	}
	a.transition(w, r, entityType, entityID, from, to, req.Action, req.Notes, id.UserID)
}

func (a *App) showReviewHistory(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	sh, err := a.ownsShow(r.Context(), id, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	hist, err := a.Store.ReviewHistory(r.Context(), "show", sh.ID)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load review history"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"history": hist})
}

// reviewTransition returns the target status for an action, and whether it is
// allowed from the current status.
func reviewTransition(from, action string) (string, bool) {
	switch action {
	case "approve":
		if from == StatusReadyForReview {
			return StatusApproved, true
		}
	case "reject":
		if from == StatusReadyForReview {
			return StatusRejected, true
		}
	case "publish":
		if from == StatusApproved {
			return StatusPublished, true
		}
	case "archive":
		if from == StatusPublished || from == StatusApproved {
			return StatusArchived, true
		}
	}
	return "", false
}

func (a *App) currentStatus(r *http.Request, entityType, entityID string) (string, error) {
	if entityType == "show" {
		sh, err := a.Store.ShowByID(r.Context(), entityID)
		if err != nil {
			return "", errcodes.Missing("show not found")
		}
		return sh.Status, nil
	}
	e, err := a.Store.EpisodeByID(r.Context(), entityID)
	if err != nil {
		return "", errcodes.Missing("episode not found")
	}
	return e.Status, nil
}

// transition performs a status change, records the review event, and emits the
// domain event through the outbox, all in one transaction.
func (a *App) transition(w http.ResponseWriter, r *http.Request, entityType, entityID, from, to, action, notes, reviewerID string) {
	ctx := r.Context()
	tx, err := a.Store.Pool().Begin(ctx)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("database unavailable"))
		return
	}
	defer tx.Rollback(ctx)

	var showID string
	var cascaded []string
	if entityType == "show" {
		if err := a.Store.SetShowStatus(ctx, tx, entityID, from, to); err != nil {
			httpx.Error(w, r, errcodes.Conflicting(err.Error()))
			return
		}
		showID = entityID

		// One review action on the show carries the whole series with it: every
		// episode whose audio is ready follows the show through the same
		// transition. Episodes still processing or failed stay behind.
		if sources := episodeCascadeSources(to); len(sources) > 0 {
			moved, err := a.Store.CascadeEpisodeStatus(ctx, tx, entityID, sources, to)
			if err != nil {
				httpx.Error(w, r, errcodes.Unexpected("could not update episodes"))
				return
			}
			cascaded = moved
		}
		if to == StatusPublished {
			if err := a.Store.RecalcShowAggregates(ctx, tx, showID); err != nil {
				httpx.Error(w, r, errcodes.Unexpected("could not update show totals"))
				return
			}
		}
	} else {
		if err := a.Store.SetEpisodeStatus(ctx, tx, entityID, from, to); err != nil {
			httpx.Error(w, r, errcodes.Conflicting(err.Error()))
			return
		}
		e, _ := a.Store.EpisodeByID(ctx, entityID)
		showID = e.ShowID
		if to == StatusPublished {
			if err := a.Store.RecalcShowAggregates(ctx, tx, showID); err != nil {
				httpx.Error(w, r, errcodes.Unexpected("could not update show totals"))
				return
			}
		}
	}

	fromCopy := from
	if err := a.Store.RecordReview(ctx, tx, ReviewEvent{
		EntityType: entityType, EntityID: entityID, ReviewerID: reviewerID,
		Action: action, Notes: notes, FromStatus: &fromCopy, ToStatus: to,
	}); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not record review"))
		return
	}

	if evType := publishedEventType(entityType, to); evType != "" {
		payload := a.publicationPayload(ctx, entityType, entityID, showID)
		if err := a.enqueue(ctx, tx, evType, 1, showID, payload); err != nil {
			httpx.Error(w, r, errcodes.Unexpected("could not record publication event"))
			return
		}
	}

	// When a show is published its cascaded episodes go live in the same
	// transaction, so each one needs its own content.episode_published event for
	// the recommendation and catalog projections.
	if entityType == "show" && to == StatusPublished {
		for _, epID := range cascaded {
			payload := a.publicationPayload(ctx, "episode", epID, showID)
			if err := a.enqueue(ctx, tx, "content.episode_published", 1, showID, payload); err != nil {
				httpx.Error(w, r, errcodes.Unexpected("could not record publication event"))
				return
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not complete transition"))
		return
	}
	metric("review_"+action, "success")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"entity_type": entityType, "entity_id": entityID, "status": to, "changed_at": time.Now().UTC(),
	})
}

// episodeCascadeSources lists the episode statuses that should follow a show
// into the target status. It mirrors reviewTransition, one step behind: an
// episode is only carried along if it is sitting in the state the show just
// left.
func episodeCascadeSources(to string) []string {
	switch to {
	case StatusReadyForReview:
		return []string{StatusDraft, StatusRejected}
	case StatusApproved, StatusRejected:
		return []string{StatusReadyForReview}
	case StatusPublished:
		return []string{StatusApproved}
	case StatusArchived:
		return []string{StatusPublished, StatusApproved}
	}
	return nil
}

func publishedEventType(entityType, to string) string {
	if to != StatusPublished {
		return ""
	}
	if entityType == "show" {
		return "content.show_published"
	}
	return "content.episode_published"
}

func (a *App) publicationPayload(ctx context.Context, entityType, entityID, showID string) map[string]any {
	sh, _ := a.Store.ShowByID(ctx, showID)
	base := map[string]any{
		"show_id": sh.ID, "title": sh.Title, "slug": sh.Slug,
		"language_code": sh.LanguageCode, "genre_ids": sh.GenreIDs, "tags": sh.Tags,
		"is_premium": sh.IsPremium, "ai_generated": sh.AIGenerated, "creator_id": sh.CreatorID,
	}
	if entityType == "episode" {
		e, _ := a.Store.EpisodeByID(ctx, entityID)
		base["episode_id"] = e.ID
		base["episode_number"] = e.Number
		base["episode_title"] = e.Title
		base["duration_sec"] = e.DurationSec
		base["is_premium"] = e.IsPremium
	}
	return base
}
