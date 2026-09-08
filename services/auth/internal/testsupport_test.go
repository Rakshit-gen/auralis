package internal

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/auralis/auth/migrations"
	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/migrate"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testIdentitySecret = "test-identity-secret"

// testDBURL is where the auth integration tests connect. Override with
// AUTH_TEST_DATABASE_URL. If the database is unreachable the tests skip rather
// than fail, so `go test` still works on a machine without Postgres.
func testDBURL() string {
	if v := os.Getenv("AUTH_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://localhost:5432/auth_test?sslmode=disable"
}

func newTestApp(t *testing.T) (*App, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDBURL())
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		t.Skipf("auth_test database not reachable (%v); set AUTH_TEST_DATABASE_URL to run", err)
	}
	if err := migrate.Run(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`TRUNCATE users, refresh_tokens, outbox_events, login_attempts RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	t.Cleanup(pool.Close)

	app := NewApp(NewStore(pool), TokenConfig{
		Secret: "test-secret-0123456789", Issuer: "auralis-auth", Audience: "auralis",
		AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour,
	})
	return app, pool
}

func testRouter(app *App) chi.Router {
	r := chi.NewRouter()
	r.Use(authn.ServiceMiddleware(testIdentitySecret))
	app.Routes(r)
	return r
}

func newServer(app *App) *httptest.Server {
	return httptest.NewServer(testRouter(app))
}

// identityHeaders returns the signed gateway headers for a caller.
func identityHeaders(userID string, roles ...string) map[string]string {
	return authn.IdentityHeaders(testIdentitySecret, authn.Identity{UserID: userID, Roles: roles}, "test-req")
}
