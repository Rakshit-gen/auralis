// Command user is the Auralis user service: profiles, preferences, likes,
// bookmarks, follows, listening history, and entitlements.
package main

import (
	"context"
	"os"

	"github.com/auralis/platform/config"
	"github.com/auralis/platform/db"
	"github.com/auralis/platform/health"
	"github.com/auralis/platform/kafkax"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/migrate"
	"github.com/auralis/platform/outbox"
	"github.com/auralis/platform/server"
	"github.com/auralis/platform/telemetry"
	"github.com/auralis/user/internal"
	"github.com/auralis/user/migrations"
)

const version = "1.0.0"

func main() {
	log := logging.Setup("user", os.Getenv("LOG_LEVEL"))
	ctx := context.Background()

	c := config.New()
	dbURL := c.Require("USER_DATABASE_URL")
	identitySecret := c.Require("IDENTITY_SECRET")
	serviceToken := c.Require("SERVICE_SHARED_TOKEN")
	kafkaBrokers := c.Require("KAFKA_BROKERS")
	addr := c.Optional("USER_HTTP_ADDR", ":8083")
	otlp := c.Optional("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	corsOrigins := c.CSV("CORS_ALLOWED_ORIGINS", nil)
	runConsumer := c.OptionalBool("USER_RUN_CONSUMER", true)
	if err := c.Err(); err != nil {
		server.FailFast("invalid configuration", err)
	}

	shutdownTracing, err := telemetry.SetupTracing(ctx, "user", version, otlp)
	if err != nil {
		log.Warn("tracing setup failed", "error", err.Error())
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer shutdownTracing(context.Background())

	pool, err := db.Open(ctx, dbURL, 12)
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

	producer := kafkax.NewProducer(kafkaBrokers, "user")
	defer producer.Close()
	bgCtx, stopBg := context.WithCancel(ctx)
	defer stopBg()
	go outbox.NewRelay(pool, producer, "user").Run(bgCtx)

	store := internal.NewStore(pool)
	app := internal.NewApp(store, serviceToken)

	if runConsumer {
		consumer := kafkax.NewConsumer(kafkax.ConsumerConfig{
			BrokerCSV: kafkaBrokers,
			GroupID:   internal.ConsumerGroup,
			Topics:    []string{kafkax.TopicUserEvents, kafkax.TopicPlaybackEvents},
			Service:   "user",
		}, store)
		go func() {
			if err := consumer.Run(bgCtx, app.Handle); err != nil {
				log.Error("consumer stopped", "error", err.Error())
			}
		}()
	}

	reg := health.New("user", version)
	reg.Add("database", db.ReadyCheck(pool))

	r := server.New(server.Options{
		Service:        "user",
		Version:        version,
		CORSOrigins:    corsOrigins,
		IdentitySecret: identitySecret,
	}, reg)
	app.Routes(r)

	if err := server.Run(ctx, addr, r); err != nil {
		server.FailFast("server error", err)
	}
}
