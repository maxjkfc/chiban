package sticker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

var errNoRows = sql.ErrNoRows

type store struct {
	db *sql.DB
}

// storedSticker is where the bytes actually are. It never leaves the package.
type storedSticker struct {
	OwnerID     uuid.UUID
	Bucket      string
	ObjectName  string
	ContentType string
}

func (s *store) insert(
	ctx context.Context,
	userID uuid.UUID,
	stickerType string,
	object storage.Object,
	sizeBytes int,
) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO user_stickers (user_id, media_type, bucket, object_name, content_type, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, userID, stickerType, object.Bucket, object.Name, object.ContentType, sizeBytes).Scan(&id)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("sticker: insert: %w", err)
	}
	return id, nil
}

func (s *store) listForOwner(ctx context.Context, userID uuid.UUID) ([]Sticker, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, media_type FROM user_stickers
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("sticker: list: %w", err)
	}
	defer rows.Close()

	stickers := []Sticker{}
	for rows.Next() {
		var one Sticker
		if err := rows.Scan(&one.ID, &one.Type); err != nil {
			return nil, fmt.Errorf("sticker: scan: %w", err)
		}
		stickers = append(stickers, one)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sticker: list: %w", err)
	}
	return stickers, nil
}

// softDelete carries the ownership check in its WHERE clause, so someone
// else's sticker matches nothing and is reported as missing rather than
// refused — the same answer an id that never existed gets.
func (s *store) softDelete(ctx context.Context, stickerID, userID uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE user_stickers SET deleted_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, stickerID, userID)
	if err != nil {
		return fmt.Errorf("sticker: delete: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sticker: delete: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *store) isLiveOwner(ctx context.Context, stickerID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_stickers
			WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		)
	`, stickerID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sticker: check owner: %w", err)
	}
	return exists, nil
}

// find ignores deleted_at on purpose: a sticker sent before it was deleted has
// to keep rendering in the conversation it was sent to.
func (s *store) find(ctx context.Context, stickerID uuid.UUID) (storedSticker, error) {
	var one storedSticker
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, bucket, object_name, content_type
		FROM user_stickers WHERE id = $1
	`, stickerID).Scan(&one.OwnerID, &one.Bucket, &one.ObjectName, &one.ContentType)
	if errors.Is(err, errNoRows) {
		return storedSticker{}, ErrNotFound
	}
	if err != nil {
		return storedSticker{}, fmt.Errorf("sticker: find: %w", err)
	}
	return one, nil
}
