package internal

import (
	"context"
	"net/http"
	"strings"

	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/httpx"
	"github.com/go-chi/chi/v5"
)

// These endpoints are called service-to-service (ai-media driving the catalog
// during AI generation) and are protected by a shared service token, not user
// identity. They are mounted under /internal/authoring.

type internalCreateShowReq struct {
	CreatorUserID string   `json:"creator_user_id"`
	CreatorName   string   `json:"creator_name"`
	Title         string   `json:"title"`
	Synopsis      string   `json:"synopsis"`
	Description   string   `json:"description"`
	LanguageCode  string   `json:"language_code"`
	GenreIDs      []string `json:"genre_ids"`
	Tags          []string `json:"tags"`
	Maturity      string   `json:"maturity"`
	AccentColor   string   `json:"accent_color"`
	CoverImageURL string   `json:"cover_image_url"`
	IsPremium     bool     `json:"is_premium"`
}

func (a *App) internalCreateShow(w http.ResponseWriter, r *http.Request) {
	var req internalCreateShowReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.CreatorUserID == "" || strings.TrimSpace(req.Title) == "" || req.LanguageCode == "" {
		httpx.Error(w, r, errcodes.BadRequest("creator_user_id, title and language_code are required"))
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
	creator, err := a.Store.UpsertCreator(ctx, req.CreatorUserID, orDefault(req.CreatorName, "Auralis Studio"))
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not resolve creator"))
		return
	}
	sh, err := a.Store.CreateShow(ctx, Show{
		CreatorID: creator.ID, Title: req.Title, Slug: a.uniqueShowSlug(ctx, req.Title),
		Synopsis: req.Synopsis, Description: req.Description, LanguageCode: req.LanguageCode,
		GenreIDs: req.GenreIDs, Tags: normalizeTags(req.Tags), Maturity: req.Maturity,
		AccentColor: req.AccentColor, CoverImageURL: req.CoverImageURL, IsPremium: req.IsPremium,
		AIGenerated: true,
	})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not create show: "+err.Error()))
		return
	}
	httpx.JSON(w, http.StatusCreated, sh)
}

type internalCreateEpisodeReq struct {
	ShowID         string `json:"show_id"`
	SeasonNumber   int    `json:"season_number"`
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Synopsis       string `json:"synopsis"`
	Script         string `json:"script"`
	AIJobID        string `json:"ai_job_id"`
	IsPremium      bool   `json:"is_premium"`
	FreePreviewSec int    `json:"free_preview_sec"`
}

func (a *App) internalCreateEpisode(w http.ResponseWriter, r *http.Request) {
	var req internalCreateEpisodeReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.ShowID == "" || req.Number < 1 || strings.TrimSpace(req.Title) == "" {
		httpx.Error(w, r, errcodes.BadRequest("show_id, number and title are required"))
		return
	}
	ctx := r.Context()
	if _, err := a.Store.ShowByID(ctx, req.ShowID); err != nil {
		httpx.Error(w, r, errcodes.Missing("show not found"))
		return
	}
	if req.SeasonNumber < 1 {
		req.SeasonNumber = 1
	}
	season, err := a.ensureSeason(ctx, req.ShowID, req.SeasonNumber)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not resolve season"))
		return
	}
	var jobID *string
	if req.AIJobID != "" {
		jobID = &req.AIJobID
	}
	e, err := a.Store.CreateEpisode(ctx, Episode{
		ShowID: req.ShowID, SeasonID: season.ID, Number: req.Number, Title: req.Title,
		Slug: slugify(req.Title), Synopsis: req.Synopsis, Script: req.Script,
		AIGenerated: true, AIJobID: jobID, IsPremium: req.IsPremium,
		FreePreviewSec: req.FreePreviewSec, Processing: ProcQueued,
	})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not create episode: "+err.Error()))
		return
	}
	if err := a.Store.RefreshEpisodeCount(ctx, req.ShowID); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not update show episode count"))
		return
	}
	httpx.JSON(w, http.StatusCreated, e)
}

type internalUpdateEpisodeReq struct {
	Script          *string        `json:"script"`
	Processing      *string        `json:"processing"`
	ProcessingError *string        `json:"processing_error"`
	Media           *MediaMetadata `json:"media"`
}

// internalUpdateEpisode lets the ai-media pipeline attach a script, advance the
// processing state, or attach packaged media metadata.
func (a *App) internalUpdateEpisode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req internalUpdateEpisodeReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	ctx := r.Context()
	e, err := a.Store.EpisodeByID(ctx, id)
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("episode not found"))
		return
	}

	if req.Media != nil {
		if err := validateMedia(*req.Media); err != nil {
			httpx.Error(w, r, err)
			return
		}
		if err := a.Store.AttachMedia(ctx, e.ID, *req.Media); err != nil {
			httpx.Error(w, r, errcodes.Unexpected("could not attach media"))
			return
		}
		metric("media_attached", "success")
	}
	if req.Script != nil {
		if _, err := a.Store.UpdateEpisode(ctx, e.ID, map[string]any{"script": *req.Script}); err != nil {
			httpx.Error(w, r, errcodes.Unexpected("could not save script"))
			return
		}
	}
	if req.Processing != nil {
		errMsg := ""
		if req.ProcessingError != nil {
			errMsg = *req.ProcessingError
		}
		if err := a.Store.SetEpisodeProcessing(ctx, e.ID, *req.Processing, errMsg); err != nil {
			httpx.Error(w, r, errcodes.BadRequest("invalid processing state"))
			return
		}
	}
	updated, _ := a.Store.EpisodeByID(ctx, e.ID)
	httpx.JSON(w, http.StatusOK, updated)
}

func (a *App) ensureSeason(ctx context.Context, showID string, number int) (Season, error) {
	return a.Store.EnsureSeason(ctx, showID, number)
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func validateMedia(m MediaMetadata) error {
	if m.HLSMasterKey == "" || len(m.Variants) < 3 {
		return errcodes.BadRequest("media must have an HLS master and at least three bitrate variants")
	}
	if m.DurationSec <= 0 || m.SampleRateHz <= 0 || m.Channels <= 0 {
		return errcodes.BadRequest("media metadata is incomplete")
	}
	have := map[int]bool{}
	for _, v := range m.Variants {
		have[v.BitrateKbps] = true
	}
	for _, want := range []int{64, 128, 256} {
		if !have[want] {
			return errcodes.BadRequest("media is missing the required bitrate variants (64, 128, 256 kbps)")
		}
	}
	return nil
}
