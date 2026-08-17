package meal

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

// DBTX is the slice of database/sql this package needs.
type DBTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// storedPhoto is the internal view of a photo, including where it actually
// lives. Nothing outside this package sees bucket or object name.
type storedPhoto struct {
	ID          uuid.UUID
	Bucket      string
	ObjectName  string
	ContentType string
}

type store struct {
	db DBTX
}

func (s *store) createMeal(ctx context.Context, userID uuid.UUID, in Input) (Meal, error) {
	m := Meal{UserID: userID}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO meal_records (user_id, meal_type, eaten_at, description)
		VALUES ($1, nullif($2, ''), $3, nullif($4, ''))
		RETURNING id, coalesce(meal_type, ''), eaten_at, coalesce(description, '')
	`, userID, in.MealType, in.EatenAt, in.Description).
		Scan(&m.ID, &m.MealType, &m.EatenAt, &m.Description)
	if err != nil {
		return Meal{}, fmt.Errorf("meal: create: %w", err)
	}
	return m, nil
}

func (s *store) createPhoto(ctx context.Context, mealID uuid.UUID, object storage.Object, sortOrder int) (Photo, error) {
	var p Photo
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO meal_photos (meal_record_id, bucket, object_name, content_type, size_bytes, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, sort_order
	`, mealID, object.Bucket, object.Name, object.ContentType, object.Size, sortOrder).
		Scan(&p.ID, &p.SortOrder)
	if err != nil {
		return Photo{}, fmt.Errorf("meal: create photo: %w", err)
	}
	return p, nil
}

// findForReader is the authorization check for reading a meal. Slice 9 widens
// the WHERE clause to include shared groups; every reader comes through here so
// there is one place to widen.
func (s *store) findForReader(ctx context.Context, mealID, readerID uuid.UUID) (Meal, error) {
	var m Meal
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, coalesce(meal_type, ''), eaten_at, coalesce(description, '')
		FROM meal_records
		WHERE id = $1 AND deleted_at IS NULL AND user_id = $2
	`, mealID, readerID).Scan(&m.ID, &m.UserID, &m.MealType, &m.EatenAt, &m.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return Meal{}, ErrNotFound
	}
	if err != nil {
		return Meal{}, fmt.Errorf("meal: find: %w", err)
	}

	photos, err := s.listPhotos(ctx, m.ID)
	if err != nil {
		return Meal{}, err
	}
	m.Photos = photos
	return m, nil
}

func (s *store) listPhotos(ctx context.Context, mealID uuid.UUID) ([]Photo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, sort_order FROM meal_photos WHERE meal_record_id = $1 ORDER BY sort_order`, mealID)
	if err != nil {
		return nil, fmt.Errorf("meal: list photos: %w", err)
	}
	defer rows.Close()

	photos := []Photo{}
	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.SortOrder); err != nil {
			return nil, fmt.Errorf("meal: scan photo: %w", err)
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}

// findPhotoForReader resolves an application-level photo ID to its storage
// location, applying the same authorization as reading the meal itself.
func (s *store) findPhotoForReader(ctx context.Context, photoID, readerID uuid.UUID) (storedPhoto, error) {
	var p storedPhoto
	err := s.db.QueryRowContext(ctx, `
		SELECT p.id, p.bucket, p.object_name, p.content_type
		FROM meal_photos p
		JOIN meal_records m ON m.id = p.meal_record_id
		WHERE p.id = $1 AND m.deleted_at IS NULL AND m.user_id = $2
	`, photoID, readerID).Scan(&p.ID, &p.Bucket, &p.ObjectName, &p.ContentType)
	if errors.Is(err, sql.ErrNoRows) {
		return storedPhoto{}, ErrPhotoNotFound
	}
	if err != nil {
		return storedPhoto{}, fmt.Errorf("meal: find photo: %w", err)
	}
	return p, nil
}

func bytesReader(data []byte) io.Reader { return bytes.NewReader(data) }
