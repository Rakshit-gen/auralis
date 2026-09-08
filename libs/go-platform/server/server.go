// Package server builds a chi router with the standard middleware stack and
// runs an HTTP server with graceful shutdown, so each service's main.go stays
// small.
package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/health"
	"github.com/auralis/platform/httpx"
	"github.com/auralis/platform/logging"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Options configure the router.
type Options struct {
	Service        string
	Version        string
	CORSOrigins    []string
	IdentitySecret string // shared secret for gateway identity headers; "" disables the check
	RateLimitRPM   int    // 0 disables the in-process limiter
	RateLimitBurst int
}

// New returns a router with request context, security headers, CORS, optional
// rate limiting, and gateway identity verification already applied, plus the
// health endpoints mounted. Register domain routes on the returned router.
func New(opts Options, reg *health.Registry) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(httpx.RequestContext(opts.Service))
	r.Use(httpx.SecurityHeaders)
	if len(opts.CORSOrigins) > 0 {
		r.Use(httpx.CORS(opts.CORSOrigins))
	}
	if opts.RateLimitRPM > 0 {
		burst := opts.RateLimitBurst
		if burst == 0 {
			burst = opts.RateLimitRPM
		}
		r.Use(httpx.NewRateLimiter(opts.RateLimitRPM, burst).Middleware)
	}
	if opts.IdentitySecret != "" {
		r.Use(authn.ServiceMiddleware(opts.IdentitySecret))
	}
	if reg != nil {
		reg.Mount(r)
	}
	return r
}

// Run serves handler on addr until SIGINT/SIGTERM, then drains for up to 20s.
func Run(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logging.L(ctx).Info("http server listening", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logging.L(ctx).Info("shutdown signal received, draining")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// FailFast logs err and exits non-zero. Used for startup errors in main.
func FailFast(msg string, err error) {
	logging.L(context.Background()).Error(msg, "error", err.Error())
	os.Exit(1)
}
