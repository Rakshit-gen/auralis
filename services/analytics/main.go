// Command analytics is the Auralis analytics service. It consumes the Kafka
// event stream and maintains aggregate tables that the dashboards query.
package main

import (
	"context"
	"os"

	"github.com/auralis/analytics/internal"
	"github.com/auralis/analytics/migrations"
	"github.com/auralis/platform/config"
	"github.com/auralis/platform/db"
	"github.com/auralis/platform/health"
	"github.com/auralis/platform/kafkax"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/migrate"
	"github.com/auralis/platform/server"
	"github.com/auralis/platform/telemetry"
)

const version = "1.0.0"

func main() {
	log := logging.Setup("analytics", os.Getenv("LOG_LEVEL"))
	ctx := context.Background()

	c := config.New()
	dbURL := c.Require("ANALYTICS_DATABASE_URL")
	identitySecret := c.Require("IDENTITY_SECRET")
	kafkaBrokers := c.Require("KAFKA_BROKERS")
	addr := c.Optional("ANALYTICS_HTTP_ADDR", ":8087")
	otlp := c.Optional("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	corsOrigins := c.CSV("CORS_ALLOWED_ORIGINS", nil)
	runConsumer := c.OptionalBool("ANALYTICS_RUN_CONSUMER", true)
	if err := c.Err(); err != nil {
		server.FailFast("invalid configuration", err)
	}

	shutdownTracing, err := telemetry.SetupTracing(ctx, "analytics", version, otlp)
	if err != nil {
		log.Warn("tracing setup failed", "error", err.Error())
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

	bgCtx, stopBg := context.WithCancel(ctx)
	defer stopBg()

	if runConsumer {
		agg := internal.NewAggregator(pool)
		consumer := kafkax.NewConsumer(kafkax.ConsumerConfig{
			BrokerCSV: kafkaBrokers,
			GroupID:   internal.ConsumerGroup,
			Topics:    []string{kafkax.TopicUserEvents, kafkax.TopicContentEvents, kafkax.TopicPlaybackEvents},
			Service:   "analytics",
		}, nil) // idempotency is handled inside Aggregator.Handle in one transaction
		go func() {
			if err := consumer.Run(bgCtx, agg.Handle); err != nil {
				log.Error("consumer stopped", "error", err.Error())
			}
		}()
	}

	reg := health.New("analytics", version)
	reg.Add("database", db.ReadyCheck(pool))

	r := server.New(server.Options{
		Service: "analytics", Version: version,
		CORSOrigins: corsOrigins, IdentitySecret: identitySecret,
	}, reg)
	internal.NewAPI(pool).Routes(r)

	if err := server.Run(ctx, addr, r); err != nil {
		server.FailFast("server error", err)
	}
}
