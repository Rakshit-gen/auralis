// Command auth is the Auralis authentication service: registration, login,
// JWT issuance, refresh token rotation, and admin user management.
package main

import (
	"context"
	"os"
	"time"

	"github.com/auralis/auth/internal"
	"github.com/auralis/auth/migrations"
	"github.com/auralis/platform/config"
	"github.com/auralis/platform/db"
	"github.com/auralis/platform/health"
	"github.com/auralis/platform/kafkax"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/migrate"
	"github.com/auralis/platform/outbox"
	"github.com/auralis/platform/server"
	"github.com/auralis/platform/telemetry"
)

const version = "1.0.0"

func main() {
	log := logging.Setup("auth", os.Getenv("LOG_LEVEL"))
	ctx := context.Background()

	c := config.New()
	dbURL := c.Require("AUTH_DATABASE_URL")
	jwtSecret := c.Require("JWT_SECRET")
	identitySecret := c.Require("IDENTITY_SECRET")
	kafkaBrokers := c.Require("KAFKA_BROKERS")
	addr := c.Optional("AUTH_HTTP_ADDR", ":8081")
	issuer := c.Optional("JWT_ISSUER", "auralis-auth")
	audience := c.Optional("JWT_AUDIENCE", "auralis")
	otlp := c.Optional("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	corsOrigins := c.CSV("CORS_ALLOWED_ORIGINS", nil)
	accessTTL := c.OptionalDuration("ACCESS_TOKEN_TTL", 15*time.Minute)
	refreshTTL := c.OptionalDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour)
	if err := c.Err(); err != nil {
		server.FailFast("invalid configuration", err)
	}

	shutdownTracing, err := telemetry.SetupTracing(ctx, "auth", version, otlp)
	if err != nil {
		log.Warn("tracing setup failed, continuing without it", "error", err.Error())
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer shutdownTracing(context.Background())

	pool, err := db.Open(ctx, dbURL, 10)
	if err != nil {
		server.FailFast("cannot connect to database", err)
	}
	defer pool.Close()

	if err := migrate.Run(ctx, pool, migrations.FS, "."); err != nil {
		server.FailFast("migrations failed", err)
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		log.Info("migrations applied, exiting")
		return
	}

	producer := kafkax.NewProducer(kafkaBrokers, "auth")
	defer producer.Close()
	relay := outbox.NewRelay(pool, producer, "auth")
	relayCtx, stopRelay := context.WithCancel(ctx)
	defer stopRelay()
	go relay.Run(relayCtx)

	store := internal.NewStore(pool)
	app := internal.NewApp(store, internal.TokenConfig{
		Secret: jwtSecret, Issuer: issuer, Audience: audience,
		AccessTTL: accessTTL, RefreshTTL: refreshTTL,
	})

	if adminEmail := c.Optional("AUTH_BOOTSTRAP_ADMIN_EMAIL", ""); adminEmail != "" {
		pw := c.Optional("AUTH_BOOTSTRAP_ADMIN_PASSWORD", "")
		name := c.Optional("AUTH_BOOTSTRAP_ADMIN_NAME", "Auralis Admin")
		if err := app.BootstrapAdmin(ctx, adminEmail, pw, name); err != nil {
			log.Error("admin bootstrap failed", "error", err.Error())
		} else {
			log.Info("admin bootstrap ensured", "email", adminEmail)
		}
	}

	reg := health.New("auth", version)
	reg.Add("database", db.ReadyCheck(pool))

	r := server.New(server.Options{
		Service:        "auth",
		Version:        version,
		CORSOrigins:    corsOrigins,
		IdentitySecret: identitySecret,
		RateLimitRPM:   c.OptionalInt("AUTH_RATE_LIMIT_RPM", 120),
		RateLimitBurst: c.OptionalInt("AUTH_RATE_LIMIT_BURST", 40),
	}, reg)
	app.Routes(r)

	if err := server.Run(ctx, addr, r); err != nil {
		server.FailFast("server error", err)
	}
}
