package internal

import (
	"net/http"
	"time"

	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/httpx"
	"github.com/go-chi/chi/v5"
)

func (a *App) listGenres(w http.ResponseWriter, r *http.Request) {
	gs, err := a.Store.Genres(r.Context())
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load genres"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"genres": gs})
}

func (a *App) listLanguages(w http.ResponseWriter, r *http.Request) {
	ls, err := a.Store.Languages(r.Context())
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load languages"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"languages": ls})
}

func (a *App) listShows(w http.ResponseWriter, r *http.Request) {
	f := ShowFilter{
		GenreSlug:    r.URL.Query().Get("genre"),
		LanguageCode: r.URL.Query().Get("language"),
		Premium:      boolPtrParam(r, "premium"),
		AIGenerated:  boolPtrParam(r, "ai"),
		Sort:         r.URL.Query().Get("sort"),
		Query:        r.URL.Query().Get("q"),
		Limit:        clampInt(queryInt(r, "limit", 24), 1, 100),
		Offset:       clampInt(queryInt(r, "offset", 0), 0, 100000),
	}
	shows, total, err := a.Store.ListShows(r.Context(), f)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not list shows"))
		return
	}
	if f.Query != "" {
		metric("search", "success")
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"shows": nonNil(shows), "total": total, "limit": f.Limit, "offset": f.Offset,
	})
}

func (a *App) getShow(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "slug")
	sh, err := a.Store.ShowByIDOrSlug(r.Context(), key)
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("show not found"))
		return
	}
	if sh.Status != StatusPublished {
		httpx.Error(w, r, errcodes.Missing("show not found"))
		return
	}
	seasons, _ := a.Store.SeasonsForShow(r.Context(), sh.ID, true)
	episodes, _ := a.Store.EpisodesForShow(r.Context(), sh.ID, true)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"show": sh, "seasons": nonNilSeasons(seasons), "episodes": publicEpisodes(episodes),
	})
}

func (a *App) getShowSeasons(w http.ResponseWriter, r *http.Request) {
	seasons, err := a.Store.SeasonsForShow(r.Context(), chi.URLParam(r, "id"), true)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load seasons"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"seasons": nonNilSeasons(seasons)})
}

func (a *App) getShowEpisodes(w http.ResponseWriter, r *http.Request) {
	eps, err := a.Store.EpisodesForShow(r.Context(), chi.URLParam(r, "id"), true)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load episodes"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"episodes": publicEpisodes(eps)})
}

func (a *App) getSeasonEpisodes(w http.ResponseWriter, r *http.Request) {
	eps, err := a.Store.EpisodesForSeason(r.Context(), chi.URLParam(r, "id"), true)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load episodes"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"episodes": publicEpisodes(eps)})
}

func (a *App) getEpisode(w http.ResponseWriter, r *http.Request) {
	e, err := a.Store.EpisodeByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil || e.Status != StatusPublished {
		httpx.Error(w, r, errcodes.Missing("episode not found"))
		return
	}
	httpx.JSON(w, http.StatusOK, publicEpisode(e))
}

func (a *App) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		httpx.Error(w, r, errcodes.BadRequest("q must be at least 2 characters"))
		return
	}
	start := time.Now()
	limit := clampInt(queryInt(r, "limit", 20), 1, 50)
	shows, total, err := a.Store.ListShows(r.Context(), ShowFilter{Query: q, Limit: limit})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("search failed"))
		return
	}
	metric("search", "success")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"query": q, "shows": nonNil(shows), "total": total,
		"took_ms": time.Since(start).Milliseconds(),
	})
}

// --- internal endpoints (called by other services with the shared service token) ---

func (a *App) getEpisodeInternal(w http.ResponseWriter, r *http.Request) {
	e, err := a.Store.EpisodeByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("episode not found"))
		return
	}
	sh, _ := a.Store.ShowByID(r.Context(), e.ShowID)
	httpx.JSON(w, http.StatusOK, map[string]any{"episode": e, "show": sh})
}

func (a *App) getShowInternal(w http.ResponseWriter, r *http.Request) {
	sh, err := a.Store.ShowByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("show not found"))
		return
	}
	httpx.JSON(w, http.StatusOK, sh)
}

// --- projection helpers: never leak scripts or internal media keys publicly ---

func publicEpisode(e Episode) Episode {
	e.Script = ""
	e.HLSMasterKey = ""
	e.ChecksumSHA256 = ""
	for i := range e.AudioVariants {
		e.AudioVariants[i].Key = ""
	}
	return e
}

func publicEpisodes(in []Episode) []Episode {
	out := make([]Episode, 0, len(in))
	for _, e := range in {
		out = append(out, publicEpisode(e))
	}
	return out
}

func nonNil(in []Show) []Show {
	if in == nil {
		return []Show{}
	}
	return in
}

func nonNilSeasons(in []Season) []Season {
	if in == nil {
		return []Season{}
	}
	return in
}
