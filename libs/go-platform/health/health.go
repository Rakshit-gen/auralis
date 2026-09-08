// Package health mounts /health, /ready, and /metrics.
//
// Liveness (/health) only reports that the process is up; it never checks
// external dependencies, so a transient database blip does not cause an
// orchestrator to kill a healthy pod. Readiness (/ready) runs the registered
// dependency checks and returns 503 with the failing check names if any fail.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Check verifies one dependency. It should return quickly and respect ctx.
type Check func(ctx context.Context) error

// Registry holds readiness checks for a service.
type Registry struct {
	service string
	version string
	checks  map[string]Check
}

func New(service, version string) *Registry {
	return &Registry{service: service, version: version, checks: map[string]Check{}}
}

// Add registers a named readiness check.
func (r *Registry) Add(name string, c Check) { r.checks[name] = c }

// Mount attaches the endpoints to router.
func (r *Registry) Mount(router chi.Router) {
	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok", "service": r.service, "version": r.version,
		})
	})
	router.Get("/ready", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
		defer cancel()
		results := map[string]string{}
		ready := true
		for name, c := range r.checks {
			if err := c(ctx); err != nil {
				results[name] = err.Error()
				ready = false
			} else {
				results[name] = "ok"
			}
		}
		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]any{
			"status": map[bool]string{true: "ready", false: "not_ready"}[ready],
			"checks": results,
		})
	})
	router.Handle("/metrics", promhttp.Handler())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
