// Command migrate applies or rolls back database migrations.
//
//	migrate up        apply all pending migrations
//	migrate down      roll back the most recent migration
//	migrate reset     roll back everything
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/maxjkfc/chiban/apps/api/internal/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: migrate up|down|reset")
	}

	databaseURL := os.Getenv("CHIBAN_DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("CHIBAN_DATABASE_URL is required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	switch os.Args[1] {
	case "up":
		return database.Up(ctx, db)
	case "down":
		return database.Down(ctx, db)
	case "reset":
		return database.DownTo(ctx, db, 0)
	default:
		return fmt.Errorf("unknown command %q (want up|down|reset)", os.Args[1])
	}
}
