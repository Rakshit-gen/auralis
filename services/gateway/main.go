// Command gateway is the Auralis API gateway. It authenticates requests at the
// edge, forwards a signed identity to the domain services, applies per-caller
// rate limits, and reverse-proxies to the eight backend services.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/auralis/gateway/internal"
	"github.com/auralis/platform/config"
	"github.com/auralis/platform/health"
	"github.com/auralis/platform/httpx"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/server"
	"github.com/auralis/platform/telemetry"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

const version = "1.0.0"

func main() {
	log := logging.Setup("gateway", os.Getenv("LOG_LEVEL"))
	ctx := context.Background()

	c := config.New()
	jwtSecret := c.Require("JWT_SECRET")
	identitySecret := c.Require("IDENTITY_SECRET")
	addr := c.Optional("GATEWAY_HTTP_ADDR", ":8080")
	issuer := c.Optional("JWT_ISSUER", "auralis-auth")
	audience := c.Optional("JWT_AUDIENCE", "auralis")
	otlp := c.Optional("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	corsOrigins := c.CSV("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"})
	redisURL := c.Optional("REDIS_URL", "")
	rateLimit := c.OptionalInt("GATEWAY_RATE_LIMIT", 240)

	backends := map[string]string{
		"auth":           c.Require("AUTH_SERVICE_URL"),
		"user":           c.Require("USER_SERVICE_URL"),
		"content":        c.Require("CONTENT_SERVICE_URL"),
		"playback":       c.Require("PLAYBACK_SERVICE_URL"),
		"recommendation": c.Require("RECOMMENDATION_SERVICE_URL"),
		"ai-media":       c.Require("AI_MEDIA_SERVICE_URL"),
		"analytics":      c.Require("ANALYTICS_SERVICE_URL"),
	}
	if err := c.Err(); err != nil {
		server.FailFast("invalid configuration", err)
	}

	shutdownTracing, err := telemetry.SetupTracing(ctx, "gateway", version, otlp)
	if err != nil {
		log.Warn("tracing setup failed", "error", err.Error())
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer shutdownTracing(context.Background())

	var limiter internal.Limiter
	var rdb *redis.Client
	if redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			server.FailFast("invalid REDIS_URL", err)
		}
		rdb = redis.NewClient(opt)
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Warn("redis unreachable, falling back to in-memory rate limiting", "error", err.Error())
			limiter = internal.NewMemoryLimiter()
		} else {
			limiter = internal.NewRedisLimiter(rdb)
			log.Info("using redis-backed rate limiter")
		}
	} else {
		limiter = internal.NewMemoryLimiter()
	}

	gw, err := internal.New(internal.Config{
		JWTSecret: jwtSecret, JWTIssuer: issuer, JWTAudience: audience,
		IdentitySecret: identitySecret, Backends: backends, Limiter: limiter,
		RateLimit: rateLimit, RateWindow: time.Minute,
	})
	if err != nil {
		server.FailFast("could not build gateway", err)
	}

	reg := health.New("gateway", version)
	if rdb != nil {
		reg.Add("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })
	}

	r := chi.NewRouter()
	r.Use(httpx.RequestContext("gateway"))
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.CORS(corsOrigins))
	reg.Mount(r)
	r.Get("/api/status", internal.SystemStatusHandler(backends))
	r.Handle("/api/*", gw)
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"service": "auralis-gateway", "version": version})
	})

	if err := server.Run(ctx, addr, r); err != nil {
		server.FailFast("server error", err)
	}
}
