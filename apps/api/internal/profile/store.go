package profile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// DBTX is the slice of database/sql this package needs.
type DBTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type store struct {
	db DBTX
}

func (s *store) find(ctx context.Context, userID uuid.UUID) (Profile, error) {
	var p Profile
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, display_name, timezone, created_at, updated_at
		FROM profiles
		WHERE user_id = $1
	`, userID).Scan(&p.UserID, &p.DisplayName, &p.Timezone, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("profile: find: %w", err)
	}
	return p, nil
}

func (s *store) upsert(ctx context.Context, userID uuid.UUID, displayName, timezone string) (Profile, error) {
	var p Profile
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO profiles (user_id, display_name, timezone)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
			SET display_name = EXCLUDED.display_name,
			    timezone     = EXCLUDED.timezone,
			    updated_at   = now()
		RETURNING user_id, display_name, timezone, created_at, updated_at
	`, userID, displayName, timezone).Scan(
		&p.UserID, &p.DisplayName, &p.Timezone, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Profile{}, fmt.Errorf("profile: upsert: %w", err)
	}
	return p, nil
}

func (s *store) displayNames(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	names := map[uuid.UUID]string{}
	if len(userIDs) == 0 {
		return names, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, display_name FROM profiles WHERE user_id = ANY($1)
	`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("profile: display names: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id   uuid.UUID
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("profile: scan display name: %w", err)
		}
		names[id] = name
	}
	return names, rows.Err()
}
