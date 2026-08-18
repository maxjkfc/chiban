package database_test

import (
	"os"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/database"
)

// Every migration needs a working Down section, otherwise a bad migration can
// only be undone by hand on the Mac mini. This exercises the whole stack of
// migrations in both directions.
func TestMigrationsRoundTrip(t *testing.T) {
	databaseURL := os.Getenv("CHIBAN_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("CHIBAN_TEST_DATABASE_URL is not set. Run tests with `make test`.")
	}

	ctx := t.Context()
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := database.Up(ctx, db); err != nil {
		t.Fatalf("initial up: %v", err)
	}
	if err := database.DownTo(ctx, db, 0); err != nil {
		t.Fatalf("down to zero: %v", err)
	}

	var applied int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM goose_db_version WHERE version_id > 0 AND is_applied`,
	).Scan(&applied); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if applied != 0 {
		t.Fatalf("after rolling back, %d migrations still applied", applied)
	}

	// Leave the database migrated so a re-run, and any other package, starts
	// from a usable schema.
	if err := database.Up(ctx, db); err != nil {
		t.Fatalf("up after down: %v", err)
	}
}
