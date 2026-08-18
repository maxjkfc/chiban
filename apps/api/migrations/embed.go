// Package migrations embeds the SQL migration files so that the API binary,
// the migrate command and the test harness all apply the same schema.
//
// Add a new migration by creating `NNNNN_name.sql` in this directory with
// goose `-- +goose Up` / `-- +goose Down` sections. Never edit a migration
// that has already been applied outside your machine; add a new one instead.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
