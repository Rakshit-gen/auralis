// Command playback is the Auralis playback service: playback authorization,
// signed media URLs, resume progress, device state, and playback events.
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
	"github.com/auralis/platform/objectstore"
	"github.com/auralis/platform/outbox"
	"github.com/auralis/platform/server"
	"github.com/auralis/platform/svcclient"
	"github.com/auralis/platform/telemetry"
	"github.com/auralis/playback/internal"
	"github.com/auralis/playback/migrations"
)

const version = "1.0.0"

func main() {
	log := logging.Setup("playback", os.Getenv("LOG_LEVEL"))
	ctx := context.Background()

	c := config.New()
	dbURL := c.Require("PLAYBACK_DATABASE_URL")
	identitySecret := c.Require("IDENTITY_SECRET")
	serviceToken := c.Require("SERVICE_SHARED_TOKEN")
	kafkaBrokers := c.Require("KAFKA_BROKERS")
	contentURL := c.Require("CONTENT_SERVICE_URL")
	userURL := c.Require("USER_SERVICE_URL")
	s3Endpoint := c.Require("S3_ENDPOINT")
	s3Key := c.Require("S3_ACCESS_KEY")
	s3Secret := c.Require("S3_SECRET_KEY")
	s3Bucket := c.Optional("S3_BUCKET", "auralis-media")
	addr := c.Optional("PLAYBACK_HTTP_ADDR", ":8084")
	otlp := c.Optional("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	corsOrigins := c.CSV("CORS_ALLOWED_ORIGINS", nil)
	runConsumer := c.OptionalBool("PLAYBACK_RUN_CONSUMER", true)
	if err := c.Err(); err != nil {
		server.FailFast("invalid configuration", err)
	}

	shutdownTracing, err := telemetry.SetupTracing(ctx, "playback", version, otlp)
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

	objects, err := objectstore.New(ctx, objectstore.Config{
		Endpoint: s3Endpoint, AccessKey: s3Key, SecretKey: s3Secret,
		UseSSL: c.OptionalBool("S3_USE_SSL", false), Region: c.Optional("S3_REGION", "us-east-1"),
		Bucket: s3Bucket, PublicBaseURL: c.Optional("S3_PUBLIC_BASE_URL", ""),
	})
	if err != nil {
		server.FailFast("cannot reach object storage", err)
	}

	producer := kafkax.NewProducer(kafkaBrokers, "playback")
	defer producer.Close()
	bgCtx, stopBg := context.WithCancel(ctx)
	defer stopBg()
	go outbox.NewRelay(pool, producer, "playback").Run(bgCtx)

	store := internal.NewStore(pool)
	app := internal.NewApp(store, objects,
		svcclient.New(contentURL, serviceToken, "playback"),
		svcclient.New(userURL, serviceToken, "playback"))

	if runConsumer {
		consumer := kafkax.NewConsumer(kafkax.ConsumerConfig{
			BrokerCSV: kafkaBrokers,
			GroupID:   internal.ConsumerGroup,
			Topics:    []string{kafkax.TopicContentEvents},
			Service:   "playback",
		}, store)
		go func() {
			if err := consumer.Run(bgCtx, app.Handle); err != nil {
				log.Error("consumer stopped", "error", err.Error())
			}
		}()
	}

	reg := health.New("playback", version)
	reg.Add("database", db.ReadyCheck(pool))
	reg.Add("object_storage", objects.ReadyCheck())

	r := server.New(server.Options{
		Service: "playback", Version: version,
		CORSOrigins: corsOrigins, IdentitySecret: identitySecret,
	}, reg)
	app.Routes(r)

	if err := server.Run(ctx, addr, r); err != nil {
		server.FailFast("server error", err)
	}
}
