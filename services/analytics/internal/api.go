package internal

import (
	"context"
	"net/http"
	"time"

	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// API serves the analytics read endpoints. Aggregates only, never raw events.
type API struct{ pool *pgxpool.Pool }

func NewAPI(pool *pgxpool.Pool) *API { return &API{pool: pool} }

// Routes mounts the analytics endpoints.
//
// Per-show and per-episode performance are visible to any authenticated caller
// (a creator checks their own show, the same numbers back the public catalog).
// Platform-wide metrics and the leaderboard are ADMIN only.
func (a *API) Routes(r chi.Router) {
	r.Route("/analytics", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(authn.RequireRoles())
			r.Get("/shows/{id}", a.showPerformance)
			r.Get("/episodes/{id}", a.episodePerformance)
		})
		r.Group(func(r chi.Router) {
			r.Use(authn.RequireRoles(authn.RoleAdmin))
			r.Get("/overview", a.overview)
			r.Get("/shows/top", a.topShows)
			r.Get("/retention", a.retention)
		})
	})
}

func (a *API) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	today := time.Now().UTC().Truncate(24 * time.Hour)

	var dau, mau int
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM listener_days WHERE day = $1`, today).Scan(&dau)
	_ = a.pool.QueryRow(ctx,
		`SELECT count(DISTINCT user_id) FROM listener_days WHERE day > $1`,
		today.AddDate(0, 0, -30)).Scan(&mau)

	var minutes, plays, completes, skips int64
	_ = a.pool.QueryRow(ctx,
		`SELECT coalesce(sum(listening_seconds)/60, 0), coalesce(sum(plays),0),
			coalesce(sum(completes),0), coalesce(sum(skips),0)
		 FROM daily_totals WHERE day > $1`, today.AddDate(0, 0, -30)).
		Scan(&minutes, &plays, &completes, &skips)

	var uniqueListeners int
	_ = a.pool.QueryRow(ctx, `SELECT count(DISTINCT user_id) FROM show_listeners`).Scan(&uniqueListeners)

	completionRate, skipRate := 0.0, 0.0
	if plays > 0 {
		completionRate = float64(completes) / float64(plays)
		skipRate = float64(skips) / float64(plays)
	}
	avgListen := 0.0
	if completes > 0 {
		avgListen = float64(minutes) / float64(completes)
	}

	daily := a.dailySeries(ctx, today.AddDate(0, 0, -14))

	httpx.JSON(w, http.StatusOK, map[string]any{
		"dau": dau, "mau": mau, "unique_listeners": uniqueListeners,
		"listening_minutes_30d": minutes, "plays_30d": plays,
		"completion_rate": round(completionRate), "skip_rate": round(skipRate),
		"avg_listen_minutes": round(avgListen),
		"daily":              daily,
	})
}

func (a *API) dailySeries(ctx context.Context, since time.Time) []map[string]any {
	rows, err := a.pool.Query(ctx,
		`SELECT d.day,
			(SELECT count(*) FROM listener_days l WHERE l.day = d.day) AS listeners,
			d.listening_seconds / 60 AS minutes, d.plays, d.completes
		 FROM daily_totals d WHERE d.day >= $1 ORDER BY d.day`, since.Truncate(24*time.Hour))
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var day time.Time
		var listeners, minutes, plays, completes int64
		if err := rows.Scan(&day, &listeners, &minutes, &plays, &completes); err == nil {
			out = append(out, map[string]any{
				"date": day.Format("2006-01-02"), "listeners": listeners,
				"listening_minutes": minutes, "plays": plays, "completes": completes,
			})
		}
	}
	return out
}

func (a *API) topShows(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pool.Query(r.Context(),
		`SELECT s.show_id, s.title, s.plays, s.completes, s.listening_seconds, s.likes, s.bookmarks, s.follows,
			(SELECT count(*) FROM show_listeners l WHERE l.show_id = s.show_id) AS unique_listeners
		 FROM show_stats s ORDER BY s.plays DESC, s.listening_seconds DESC LIMIT 20`)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load show leaderboard"))
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, title string
		var plays, completes, secs, likes, bookmarks, follows, listeners int64
		if err := rows.Scan(&id, &title, &plays, &completes, &secs, &likes, &bookmarks, &follows, &listeners); err != nil {
			httpx.Error(w, r, errcodes.Unexpected("could not read show stats"))
			return
		}
		out = append(out, map[string]any{
			"show_id": id, "title": title, "plays": plays, "completes": completes,
			"listening_minutes": secs / 60, "unique_listeners": listeners,
			"likes": likes, "bookmarks": bookmarks, "follows": follows,
			"completion_rate": ratio(completes, plays),
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"shows": out})
}

func (a *API) showPerformance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	var title string
	var plays, completes, secs, likes, bookmarks, follows int64
	err := a.pool.QueryRow(ctx,
		`SELECT title, plays, completes, listening_seconds, likes, bookmarks, follows
		 FROM show_stats WHERE show_id = $1`, id).
		Scan(&title, &plays, &completes, &secs, &likes, &bookmarks, &follows)
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("no analytics for this show yet"))
		return
	}
	var listeners int
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM show_listeners WHERE show_id = $1`, id).Scan(&listeners)

	epRows, _ := a.pool.Query(ctx,
		`SELECT episode_id, plays, completes, skips, listening_seconds,
			CASE WHEN completion_ratio_n = 0 THEN 0 ELSE completion_ratio_sum / completion_ratio_n END
		 FROM episode_stats WHERE show_id = $1 ORDER BY plays DESC`, id)
	var episodes []map[string]any
	if epRows != nil {
		defer epRows.Close()
		for epRows.Next() {
			var eid string
			var ep, ec, es, esec int64
			var cr float64
			if err := epRows.Scan(&eid, &ep, &ec, &es, &esec, &cr); err == nil {
				episodes = append(episodes, map[string]any{
					"episode_id": eid, "plays": ep, "completes": ec, "skips": es,
					"listening_minutes": esec / 60, "avg_completion": round(cr),
				})
			}
		}
	}
	if episodes == nil {
		episodes = []map[string]any{}
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"show_id": id, "title": title, "plays": plays, "completes": completes,
		"unique_listeners": listeners, "listening_minutes": secs / 60,
		"likes": likes, "bookmarks": bookmarks, "follows": follows,
		"completion_rate": ratio(completes, plays), "episodes": episodes,
	})
}

func (a *API) episodePerformance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var showID string
	var plays, completes, skips, secs, n int64
	var ratioSum float64
	err := a.pool.QueryRow(r.Context(),
		`SELECT show_id, plays, completes, skips, listening_seconds, completion_ratio_sum, completion_ratio_n
		 FROM episode_stats WHERE episode_id = $1`, id).
		Scan(&showID, &plays, &completes, &skips, &secs, &ratioSum, &n)
	if err != nil {
		httpx.Error(w, r, errcodes.Missing("no analytics for this episode yet"))
		return
	}
	avgCompletion := 0.0
	if n > 0 {
		avgCompletion = ratioSum / float64(n)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"episode_id": id, "show_id": showID, "plays": plays, "completes": completes, "skips": skips,
		"listening_minutes": secs / 60, "completion_rate": ratio(completes, plays),
		"avg_completion": round(avgCompletion), "skip_rate": ratio(skips, plays),
	})
}

func (a *API) retention(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pool.Query(r.Context(),
		`SELECT r.cohort_week,
			(SELECT count(*) FROM user_cohorts c WHERE c.cohort_week = r.cohort_week) AS cohort_size,
			r.activity_week, count(DISTINCT r.user_id) AS active
		 FROM retention r
		 WHERE r.cohort_week > now() - interval '12 weeks'
		 GROUP BY r.cohort_week, r.activity_week
		 ORDER BY r.cohort_week, r.activity_week`)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not load retention"))
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var cohort, activity time.Time
		var size, active int64
		if err := rows.Scan(&cohort, &size, &activity, &active); err != nil {
			httpx.Error(w, r, errcodes.Unexpected("could not read retention"))
			return
		}
		weekOffset := int(activity.Sub(cohort).Hours() / 24 / 7)
		out = append(out, map[string]any{
			"cohort_week": cohort.Format("2006-01-02"), "cohort_size": size,
			"week_offset": weekOffset, "active": active, "rate": ratio(active, size),
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"cohorts": out})
}

func ratio(num, den int64) float64 {
	if den == 0 {
		return 0
	}
	return round(float64(num) / float64(den))
}

func round(f float64) float64 {
	return float64(int(f*10000+0.5)) / 10000
}
