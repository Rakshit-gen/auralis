package internal

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/httpx"
	"github.com/go-chi/chi/v5"
)

type createShowReq struct {
	Title         string   `json:"title"`
	Synopsis      string   `json:"synopsis"`
	Description   string   `json:"description"`
	LanguageCode  string   `json:"language_code"`
	GenreIDs      []string `json:"genre_ids"`
	Tags          []string `json:"tags"`
	Maturity      string   `json:"maturity"`
	CoverImageURL string   `json:"cover_image_url"`
	AccentColor   string   `json:"accent_color"`
	IsPremium     bool     `json:"is_premium"`
	CreatorName   string   `json:"creator_name"`
}

func (a *App) createShow(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	var req createShowReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if len(req.Title) < 2 || len(req.Title) > 120 {
		httpx.Error(w, r, errcodes.BadRequest("title must be between 2 and 120 characters"))
		return
	}
	if req.LanguageCode == "" {
		httpx.Error(w, r, errcodes.BadRequest("language_code is required"))
		return
	}
	if req.Maturity == "" {
		req.Maturity = "general"
	}
	if req.AccentColor == "" {
		req.AccentColor = "#c98a3c"
	}

	ctx := r.Context()
	if err := a.validateGenres(ctx, req.GenreIDs); err != nil {
		httpx.Error(w, r, err)
		return
	}
	creator, err := a.Store.UpsertCreator(ctx, id.UserID, creatorNameFrom(id, req.CreatorName))
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not resolve creator profile"))
		return
	}

	sh := Show{
		CreatorID: creator.ID, Title: req.Title, Slug: a.uniqueShowSlug(ctx, req.Title),
		Synopsis: req.Synopsis, Description: req.Description, LanguageCode: req.LanguageCode,
		GenreIDs: req.GenreIDs, Tags: normalizeTags(req.Tags), Maturity: req.Maturity,
		CoverImageURL: req.CoverImageURL, AccentColor: req.AccentColor, IsPremium: req.IsPremium,
	}
	created, err := a.Store.CreateShow(ctx, sh)
	if err != nil {
		if strings.Contains(err.Error(), "language") {
			httpx.Error(w, r, errcodes.BadRequest("unknown language_code"))
			return
		}
		httpx.Error(w, r, errcodes.Unexpected("could not create show"))
		return
	}
	metric("show_created", "success")
	httpx.JSON(w, http.StatusCreated, created)
}

func (a *App) listMyShows(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	creator, err := a.Store.CreatorByUser(r.Context(), id.UserID)
	if err != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"shows": []Show{}})
		return
	}
	shows, _, err := a.Store.ListShows(r.Context(), ShowFilter{
		CreatorID: creator.ID, IncludeNonPublished: true, Limit: 100, Sort: "recent",
	})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not list shows"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"shows": nonNil(shows)})
}

func (a *App) updateShow(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	sh, err := a.ownsShow(r.Context(), id, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if sh.Status != StatusDraft && sh.Status != StatusRejected {
		httpx.Error(w, r, errcodes.Conflicting("only draft or rejected shows can be edited"))
		return
	}
	var body map[string]any
	if err := httpx.Decode(w, r, &body); err != nil {
		httpx.Error(w, r, err)
		return
	}
	patch := map[string]any{}
	for _, f := range []string{"title", "synopsis", "description", "cover_image_url", "accent_color", "maturity"} {
		if v, ok := body[f].(string); ok {
			patch[f] = v
		}
	}
	if v, ok := body["is_premium"].(bool); ok {
		patch["is_premium"] = v
	}
	if raw, ok := body["tags"].([]any); ok {
		patch["tags"] = normalizeTags(toStringSlice(raw))
	}
	if raw, ok := body["genre_ids"].([]any); ok {
		gids := toStringSlice(raw)
		if err := a.validateGenres(r.Context(), gids); err != nil {
			httpx.Error(w, r, err)
			return
		}
		patch["genre_ids"] = gids
	}
	updated, err := a.Store.UpdateShow(r.Context(), sh.ID, patch)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not update show"))
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

type createSeasonReq struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (a *App) createSeason(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	sh, err := a.ownsShow(r.Context(), id, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req createSeasonReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.Number < 1 {
		httpx.Error(w, r, errcodes.BadRequest("season number must be >= 1"))
		return
	}
	if req.Title == "" {
		req.Title = fmt.Sprintf("Season %d", req.Number)
	}
	se, err := a.Store.CreateSeason(r.Context(), Season{
		ShowID: sh.ID, Number: req.Number, Title: req.Title, Description: req.Description,
	})
	if err != nil {
		if strings.Contains(err.Error(), "seasons_show_id_number_key") {
			httpx.Error(w, r, errcodes.Conflicting("a season with this number already exists"))
			return
		}
		httpx.Error(w, r, errcodes.Unexpected("could not create season"))
		return
	}
	httpx.JSON(w, http.StatusCreated, se)
}

type createEpisodeReq struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Synopsis       string `json:"synopsis"`
	Script         string `json:"script"`
	IsPremium      bool   `json:"is_premium"`
	FreePreviewSec int    `json:"free_preview_sec"`
}

func (a *App) createEpisode(w http.ResponseWriter, r *http.Request) {
	id, _ := authn.FromContext(r.Context())
	se, err := a.Store.SeasonByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("season not found"))
		return
	}
	if _, err := a.ownsShow(r.Context(), id, se.ShowID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req createEpisodeReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.Number < 1 {
		httpx.Error(w, r, errcodes.BadRequest("episode number must be >= 1"))
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		httpx.Error(w, r, errcodes.BadRequest("title is required"))
		return
	}
	e, err := a.Store.CreateEpisode(r.Context(), Episode{
		ShowID: se.ShowID, SeasonID: se.ID, Number: req.Number, Title: req.Title,
		Slug: slugify(req.Title), Synopsis: req.Synopsis, Script: req.Script,
		IsPremium: req.IsPremium, FreePreviewSec: req.FreePreviewSec,
	})
	if err != nil {
		if strings.Contains(err.Error(), "episodes_season_id_number_key") {
			httpx.Error(w, r, errcodes.Conflicting("an episode with this number already exists"))
			return
		}
		httpx.Error(w, r, errcodes.Unexpected("could not create episode"))
		return
	}
	httpx.JSON(w, http.StatusCreated, e)
}

func (a *App) updateEpisode(w http.ResponseWriter, r *http.Request) {
	e, err := a.episodeOwned(w, r)
	if err != nil {
		return
	}
	if e.Status != StatusDraft && e.Status != StatusRejected {
		httpx.Error(w, r, errcodes.Conflicting("only draft or rejected episodes can be edited"))
		return
	}
	var body map[string]any
	if err := httpx.Decode(w, r, &body); err != nil {
		httpx.Error(w, r, err)
		return
	}
	patch := map[string]any{}
	if v, ok := body["title"].(string); ok && strings.TrimSpace(v) != "" {
		patch["title"] = v
		patch["slug"] = slugify(v)
	}
	if v, ok := body["synopsis"].(string); ok {
		patch["synopsis"] = v
	}
	if v, ok := body["is_premium"].(bool); ok {
		patch["is_premium"] = v
	}
	if v, ok := body["free_preview_sec"].(float64); ok {
		patch["free_preview_sec"] = int(v)
	}
	updated, err := a.Store.UpdateEpisode(r.Context(), e.ID, patch)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not update episode"))
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

type editScriptReq struct {
	Script string `json:"script"`
}

// editScript replaces an episode's script. Allowed for AI-generated episodes
// that were rejected or are still draft, so a creator can revise before
// resubmitting.
func (a *App) editScript(w http.ResponseWriter, r *http.Request) {
	e, err := a.episodeOwned(w, r)
	if err != nil {
		return
	}
	if e.Status != StatusDraft && e.Status != StatusRejected && e.Status != StatusReadyForReview {
		httpx.Error(w, r, errcodes.Conflicting("script can only be edited before approval"))
		return
	}
	var req editScriptReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if len(strings.TrimSpace(req.Script)) < 20 {
		httpx.Error(w, r, errcodes.BadRequest("script is too short"))
		return
	}
	updated, err := a.Store.UpdateEpisode(r.Context(), e.ID, map[string]any{
		"script": req.Script, "processing": ProcNone,
	})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not save script"))
		return
	}
	metric("script_edited", "success")
	httpx.JSON(w, http.StatusOK, updated)
}

// --- helpers ---

func (a *App) episodeOwned(w http.ResponseWriter, r *http.Request) (Episode, error) {
	id, _ := authn.FromContext(r.Context())
	e, err := a.Store.EpisodeByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("episode not found"))
		return Episode{}, err
	}
	if _, err := a.ownsShow(r.Context(), id, e.ShowID); err != nil {
		httpx.Error(w, r, err)
		return Episode{}, err
	}
	return e, nil
}

// validateGenres checks every id refers to a real genre.
func (a *App) validateGenres(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	known, err := a.Store.genreMap(ctx)
	if err != nil {
		return errcodes.Unexpected("could not validate genres")
	}
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			return errcodes.BadRequest("unknown genre id: " + id)
		}
	}
	return nil
}

// uniqueShowSlug derives a slug from title and appends a suffix on collision.
func (a *App) uniqueShowSlug(ctx context.Context, title string) string {
	base := slugify(title)
	if base == "" {
		base = "show"
	}
	slug := base
	for i := 2; i < 100; i++ {
		if _, err := a.Store.ShowBySlug(ctx, slug); err == ErrNotFound {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return fmt.Sprintf("%s-%d", base, len(title))
}

func normalizeTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] && len(out) < 15 {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func toStringSlice(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
