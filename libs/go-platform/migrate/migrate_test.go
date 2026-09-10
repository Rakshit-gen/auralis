package migrate

import (
	"context"
	"os"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

func dbURL() string {
	if v := os.Getenv("MIGRATE_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://localhost:5432/auth_test?sslmode=disable"
}

// TestConcurrentRunDoesNotRace starts several migrators against the same fresh
// migration at once. Without the advisory lock they race the exists-check and
// all but one fail on "relation already exists"; with it they serialize and the
// losers simply see the version already applied.
func TestConcurrentRunDoesNotRace(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL())
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		t.Skipf("test database not reachable (%v)", err)
	}
	defer pool.Close()

	const version = "9999_migrate_race_test"
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS _migrate_race_test`)
	_, _ = pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS _migrate_race_test`)
		_, _ = pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version)
	})

	// No IF NOT EXISTS: a second uncoordinated apply of this file errors. The
	// sleep widens the exists-check/apply window so an unlocked race is reliable.
	fsys := fstest.MapFS{
		version + ".sql": {Data: []byte("SELECT pg_sleep(0.05);\nCREATE TABLE _migrate_race_test (id int primary key)")},
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = Run(ctx, pool, fsys, ".")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("migrator %d failed: %v", i, err)
		}
	}
}
