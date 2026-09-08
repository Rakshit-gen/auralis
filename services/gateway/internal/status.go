package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// SystemStatusHandler fans out to each backend's /health and /ready and returns
// a combined view. It is public: it exposes only up/down and latency, never any
// data. The admin console uses it for the system-health page.
func SystemStatusHandler(backends map[string]string) http.HandlerFunc {
	client := &http.Client{Timeout: 3 * time.Second}
	return func(w http.ResponseWriter, r *http.Request) {
		type result struct {
			Name      string `json:"name"`
			Healthy   bool   `json:"healthy"`
			Ready     bool   `json:"ready"`
			LatencyMS int64  `json:"latency_ms"`
			Detail    string `json:"detail,omitempty"`
		}

		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		out := make([]result, len(backends))
		names := make([]string, 0, len(backends))
		for n := range backends {
			names = append(names, n)
		}
		for idx, name := range names {
			wg.Add(1)
			go func(idx int, name, base string) {
				defer wg.Done()
				res := result{Name: name}
				start := time.Now()
				if req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil); err == nil {
					if resp, err := client.Do(req); err == nil {
						res.Healthy = resp.StatusCode == 200
						resp.Body.Close()
					} else {
						res.Detail = "unreachable"
					}
				}
				if req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/ready", nil); err == nil {
					if resp, err := client.Do(req); err == nil {
						res.Ready = resp.StatusCode == 200
						resp.Body.Close()
					}
				}
				res.LatencyMS = time.Since(start).Milliseconds()
				out[idx] = res
			}(idx, name, backends[name])
		}
		wg.Wait()

		allReady := true
		for _, res := range out {
			if !res.Ready {
				allReady = false
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if !allReady {
			w.WriteHeader(http.StatusMultiStatus)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"services":   out,
			"all_ready":  allReady,
			"checked_at": time.Now().UTC(),
		})
	}
}
