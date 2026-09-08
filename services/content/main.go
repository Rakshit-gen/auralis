// Command content is the Auralis catalog service: shows, seasons, episodes,
// genres, Postgres full-text search, the review workflow, and publication.
package main

import (
	"context"
	"os"

	"github.com/auralis/content/internal"
	"github.com/auralis/content/migrations"
	"github.com/auralis/platform/config"
	"github.com/auralis/platform/db"
	"github.com/auralis/platform/health"
	"github.com/auralis/platform/kafkax"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/migrate"
	"github.com/auralis/platform/objectstore"
	"github.com/auralis/platform/outbox"
	"github.com/auralis/platform/server"
	"github.com/auralis/platform/telemetry"
)

const version = "1.0.0"

func main() {
	log := logging.Setup("content", os.Getenv("LOG_LEVEL"))
	ctx := context.Background()

	c := config.New()
	dbURL := c.Require("CONTENT_DATABASE_URL")
	identitySecret := c.Require("IDENTITY_SECRET")
	serviceToken := c.Require("SERVICE_SHARED_TOKEN")
	kafkaBrokers := c.Require("KAFKA_BROKERS")
	s3Endpoint := c.Require("S3_ENDPOINT")
	s3Key := c.Require("S3_ACCESS_KEY")
	s3Secret := c.Require("S3_SECRET_KEY")
	s3Bucket := c.Optional("S3_BUCKET", "auralis-media")
	addr := c.Optional("CONTENT_HTTP_ADDR", ":8082")
	otlp := c.Optional("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	corsOrigins := c.CSV("CORS_ALLOWED_ORIGINS", nil)
	if err := c.Err(); err != nil {
		server.FailFast("invalid configuration", err)
	}

	shutdownTracing, err := telemetry.SetupTracing(ctx, "content", version, otlp)
	if err != nil {
		log.Warn("tracing setup failed", "error", err.Error())
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer shutdownTracing(context.Background())

	pool, err := db.Open(ctx, dbURL, 15)
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
		Endpoint:      s3Endpoint,
		AccessKey:     s3Key,
		SecretKey:     s3Secret,
		UseSSL:        c.OptionalBool("S3_USE_SSL", false),
		Region:        c.Optional("S3_REGION", "us-east-1"),
		Bucket:        s3Bucket,
		PublicBaseURL: c.Optional("S3_PUBLIC_BASE_URL", ""),
	})
	if err != nil {
		server.FailFast("cannot reach object storage", err)
	}

	producer := kafkax.NewProducer(kafkaBrokers, "content")
	defer producer.Close()
	relayCtx, stopRelay := context.WithCancel(ctx)
	defer stopRelay()
	go outbox.NewRelay(pool, producer, "content").Run(relayCtx)

	app := internal.NewApp(internal.NewStore(pool), objects, serviceToken)

	reg := health.New("content", version)
	reg.Add("database", db.ReadyCheck(pool))
	reg.Add("object_storage", objects.ReadyCheck())

	r := server.New(server.Options{
		Service:        "content",
		Version:        version,
		CORSOrigins:    corsOrigins,
		IdentitySecret: identitySecret,
	}, reg)
	app.Routes(r)

	if err := server.Run(ctx, addr, r); err != nil {
		server.FailFast("server error", err)
	}
}
