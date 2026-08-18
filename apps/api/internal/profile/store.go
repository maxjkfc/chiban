package profile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

// DBTX is the slice of database/sql this package needs.
type DBTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// storedAvatar is where an avatar actually lives. Nothing outside this package
// sees a bucket or an object name.
type storedAvatar struct {
	OwnerID     uuid.UUID
	Bucket      string
	ObjectName  string
	ContentType string
}

type store struct {
	db DBTX
}

func (s *store) find(ctx context.Context, userID uuid.UUID) (Profile, error) {
	var p Profile
	var avatarMediaID uuid.NullUUID
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, display_name, timezone, avatar_media_id, created_at, updated_at
		FROM profiles
		WHERE user_id = $1
	`, userID).Scan(&p.UserID, &p.DisplayName, &p.Timezone, &avatarMediaID, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("profile: find: %w", err)
	}
	p.AvatarMediaID = avatarMediaID.UUID
	return p, nil
}

func (s *store) upsert(ctx context.Context, userID uuid.UUID, displayName, timezone string) (Profile, error) {
	var (
		p             Profile
		avatarMediaID uuid.NullUUID
	)
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO profiles (user_id, display_name, timezone)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
			SET display_name = EXCLUDED.display_name,
			    timezone     = EXCLUDED.timezone,
			    updated_at   = now()
		RETURNING user_id, display_name, timezone, avatar_media_id, created_at, updated_at
	`, userID, displayName, timezone).Scan(
		&p.UserID, &p.DisplayName, &p.Timezone, &avatarMediaID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Profile{}, fmt.Errorf("profile: upsert: %w", err)
	}
	p.AvatarMediaID = avatarMediaID.UUID
	return p, nil
}

func (s *store) setAvatar(ctx context.Context, userID, mediaID uuid.UUID, object storage.Object) (Profile, error) {
	var (
		p             Profile
		avatarMediaID uuid.NullUUID
	)
	err := s.db.QueryRowContext(ctx, `
		UPDATE profiles
		SET avatar_media_id = $2, avatar_bucket = $3, avatar_object_name = $4,
		    avatar_content_type = $5, updated_at = now()
		WHERE user_id = $1
		RETURNING user_id, display_name, timezone, avatar_media_id, created_at, updated_at
	`, userID, mediaID, object.Bucket, object.Name, object.ContentType).Scan(
		&p.UserID, &p.DisplayName, &p.Timezone, &avatarMediaID, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("profile: set avatar: %w", err)
	}
	p.AvatarMediaID = avatarMediaID.UUID
	return p, nil
}

// lockAvatarForUser takes the profile row and reports where the current
// picture lives, or a zero value when there is none.
//
// FOR UPDATE holds the row for the rest of the transaction, so a second upload
// for the same user waits rather than racing this one to the same conclusion.
func (s *store) lockAvatarForUser(ctx context.Context, userID uuid.UUID) (storedAvatar, error) {
	var (
		a          storedAvatar
		bucket     sql.NullString
		objectName sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, avatar_bucket, avatar_object_name
		FROM profiles
		WHERE user_id = $1
		FOR UPDATE
	`, userID).Scan(&a.OwnerID, &bucket, &objectName)
	if errors.Is(err, sql.ErrNoRows) {
		return storedAvatar{}, ErrNotFound
	}
	if err != nil {
		return storedAvatar{}, fmt.Errorf("profile: lock avatar: %w", err)
	}
	a.Bucket, a.ObjectName = bucket.String, objectName.String
	return a, nil
}

// findAvatarObject resolves the ID the client holds to a storage location. The
// client never supplies a bucket or object name, so there is no path to
// traverse.
func (s *store) findAvatarObject(ctx context.Context, mediaID uuid.UUID) (storedAvatar, error) {
	var a storedAvatar
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, avatar_bucket, avatar_object_name, avatar_content_type
		FROM profiles
		WHERE avatar_media_id = $1
	`, mediaID).Scan(&a.OwnerID, &a.Bucket, &a.ObjectName, &a.ContentType)
	if errors.Is(err, sql.ErrNoRows) {
		return storedAvatar{}, ErrAvatarNotFound
	}
	if err != nil {
		return storedAvatar{}, fmt.Errorf("profile: find avatar: %w", err)
	}
	return a, nil
}

func (s *store) summaries(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]Summary, error) {
	summaries := map[uuid.UUID]Summary{}
	if len(userIDs) == 0 {
		return summaries, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, display_name, avatar_media_id FROM profiles WHERE user_id = ANY($1)
	`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("profile: summaries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id            uuid.UUID
			summary       Summary
			avatarMediaID uuid.NullUUID
		)
		if err := rows.Scan(&id, &summary.DisplayName, &avatarMediaID); err != nil {
			return nil, fmt.Errorf("profile: scan summary: %w", err)
		}
		summary.AvatarMediaID = avatarMediaID.UUID
		summaries[id] = summary
	}
	return summaries, rows.Err()
}
