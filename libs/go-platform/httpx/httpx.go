// Package httpx provides the shared HTTP layer: request-scoped middleware,
// JSON helpers, CORS, security headers, and a token-bucket rate limiter.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/telemetry"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// MaxBodyBytes is the default request body cap for JSON endpoints (1 MiB).
const MaxBodyBytes = 1 << 20

// RequestContext installs request id / correlation id / trace id, structured
// logging fields, panic recovery, and HTTP server metrics.
func RequestContext(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := r.Header.Get("X-Auralis-Request-Id")
			if reqID == "" {
				reqID = r.Header.Get("X-Request-Id")
			}
			if reqID == "" {
				reqID = uuid.NewString()
			}
			corrID := r.Header.Get("X-Correlation-Id")
			if corrID == "" {
				corrID = reqID
			}
			traceID := r.Header.Get("X-Trace-Id")

			r.Header.Set("X-Auralis-Request-Id", reqID)
			w.Header().Set("X-Request-Id", reqID)
			w.Header().Set("X-Correlation-Id", corrID)

			ctx := logging.WithFields(r.Context(), logging.Fields{
				RequestID: reqID, TraceID: traceID, CorrelationID: corrID, Endpoint: r.URL.Path,
			})
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				endpoint := chiRoutePattern(r) // available after routing
				if rec := recover(); rec != nil {
					logging.L(ctx).Error("panic recovered", "panic", rec, "endpoint", endpoint)
					if ww.Status() == 0 {
						errcodes.Write(ww, reqID, errcodes.Unexpected("an unexpected error occurred"))
					}
				}
				dur := time.Since(start)
				status := ww.Status()
				if status == 0 {
					status = 200
				}
				telemetry.ObserveHTTP(service, r.Method, endpoint, status, dur)
				logging.L(ctx).Info("request",
					"method", r.Method, "status", status,
					"duration_ms", float64(dur.Microseconds())/1000.0,
					"bytes", ww.BytesWritten(), "remote", clientIP(r))
			}()

			next.ServeHTTP(ww, r.WithContext(ctx))
		})
	}
}

// chiRoutePattern returns the matched route template (e.g. /shows/{id}) so
// metrics and logs group by route instead of exploding per path parameter.
func chiRoutePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	if r.URL.Path != "" {
		return r.URL.Path
	}
	return "unknown"
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	h, _, _ := strings.Cut(r.RemoteAddr, ":")
	return h
}

// SecurityHeaders sets conservative response headers on every route.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}

// CORS returns middleware allowing the configured origins with credentials.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	wildcard := false
	for _, o := range origins {
		if o == "*" {
			wildcard = true
		}
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (wildcard || allowed[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Correlation-Id")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- JSON helpers ---

// JSON writes v as a JSON response with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Decode reads and strictly decodes a JSON body into v, enforcing the size cap.
func Decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errcodes.New(http.StatusRequestEntityTooLarge, errcodes.PayloadTooBig, "request body too large")
		}
		if errors.Is(err, io.EOF) {
			return errcodes.BadRequest("request body is empty")
		}
		return errcodes.BadRequest("malformed JSON body: " + err.Error())
	}
	if dec.More() {
		return errcodes.BadRequest("request body must contain a single JSON object")
	}
	return nil
}

// Error writes err using the platform envelope, pulling the request id from ctx.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	errcodes.Write(w, logging.FromContext(r.Context()).RequestID, err)
}

// --- Rate limiter ---

// RateLimiter is a per-key token bucket kept in memory. Production deployments
// front it with the gateway's Redis limiter; this bounds a single instance.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64
	capacity float64
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter allows `perMinute` requests per key with burst `burst`.
func NewRateLimiter(perMinute, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     float64(perMinute) / 60.0,
		capacity: float64(burst),
	}
	go rl.gc()
	return rl
}

func (rl *RateLimiter) gc() {
	for range time.Tick(5 * time.Minute) {
		rl.mu.Lock()
		for k, b := range rl.buckets {
			if time.Since(b.last) > 10*time.Minute {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow reports whether a request for key may proceed now.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &bucket{tokens: rl.capacity - 1, last: now}
		return true
	}
	b.tokens = min(rl.capacity, b.tokens+now.Sub(b.last).Seconds()*rl.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Middleware rejects requests over the limit, keyed by identity or client IP.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Auralis-User")
		if key == "" {
			key = clientIP(r)
		}
		if !rl.Allow(key) {
			w.Header().Set("Retry-After", "1")
			errcodes.Write(w, r.Header.Get("X-Auralis-Request-Id"),
				errcodes.New(http.StatusTooManyRequests, errcodes.RateLimited, "rate limit exceeded"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
