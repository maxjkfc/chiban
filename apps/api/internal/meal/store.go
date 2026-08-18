package meal

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"

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

// readableMealsCTE is the one definition of "which meals may this reader see".
//
// Both the meal query and the photo query select through it, so widening it
// for meal sharing in slice 9 is a single edit. Two hand-written copies of the
// same predicate would drift, and the way they would drift is a shared-group
// member reading a meal but 404ing on its photos, or the reverse.
const readableMealsCTE = `
	WITH readable AS (
		SELECT id, user_id, meal_type, eaten_at, description
		FROM meal_records
		WHERE deleted_at IS NULL AND user_id = $2
	)
`

// findForReader is the authorization check for reading a meal.
func (s *store) findForReader(ctx context.Context, mealID, readerID uuid.UUID) (Meal, error) {
	var m Meal
	err := s.db.QueryRowContext(ctx, readableMealsCTE+`
		SELECT id, user_id, coalesce(meal_type, ''), eaten_at, coalesce(description, '')
		FROM readable
		WHERE id = $1
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

// findOwned is the authorization check for writing: unlike reading, it never
// widens to shared groups.
func (s *store) findOwned(ctx context.Context, mealID, ownerID uuid.UUID) (Meal, error) {
	var m Meal
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, coalesce(meal_type, ''), eaten_at, coalesce(description, '')
		FROM meal_records
		WHERE id = $1 AND deleted_at IS NULL AND user_id = $2
	`, mealID, ownerID).Scan(&m.ID, &m.UserID, &m.MealType, &m.EatenAt, &m.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return Meal{}, ErrNotFound
	}
	if err != nil {
		return Meal{}, fmt.Errorf("meal: find owned: %w", err)
	}
	return m, nil
}

func (s *store) listBetween(ctx context.Context, userID uuid.UUID, start, end time.Time) ([]Meal, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, coalesce(meal_type, ''), eaten_at, coalesce(description, '')
		FROM meal_records
		WHERE user_id = $1 AND deleted_at IS NULL AND eaten_at >= $2 AND eaten_at < $3
		ORDER BY eaten_at
	`, userID, start, end)
	if err != nil {
		return nil, fmt.Errorf("meal: list: %w", err)
	}
	defer rows.Close()

	meals := []Meal{}
	for rows.Next() {
		var m Meal
		if err := rows.Scan(&m.ID, &m.UserID, &m.MealType, &m.EatenAt, &m.Description); err != nil {
			return nil, fmt.Errorf("meal: scan: %w", err)
		}
		meals = append(meals, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(meals))
	for _, m := range meals {
		ids = append(ids, m.ID)
	}
	byMeal, err := s.photosFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range meals {
		meals[i].Photos = byMeal[meals[i].ID]
	}
	return meals, nil
}

// photosFor loads every meal's photos in one query. A query per meal would be
// harmless for one person's day but not for slice 9's group meal cards.
func (s *store) photosFor(ctx context.Context, mealIDs []uuid.UUID) (map[uuid.UUID][]Photo, error) {
	byMeal := map[uuid.UUID][]Photo{}
	if len(mealIDs) == 0 {
		return byMeal, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT meal_record_id, id, sort_order
		FROM meal_photos
		WHERE meal_record_id = ANY($1)
		ORDER BY meal_record_id, sort_order
	`, mealIDs)
	if err != nil {
		return nil, fmt.Errorf("meal: list photos: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			mealID uuid.UUID
			p      Photo
		)
		if err := rows.Scan(&mealID, &p.ID, &p.SortOrder); err != nil {
			return nil, fmt.Errorf("meal: scan photo: %w", err)
		}
		byMeal[mealID] = append(byMeal[mealID], p)
	}
	return byMeal, rows.Err()
}

func (s *store) update(ctx context.Context, mealID, ownerID uuid.UUID, in Input) (Meal, error) {
	var m Meal
	err := s.db.QueryRowContext(ctx, `
		UPDATE meal_records
		SET meal_type = nullif($3, ''), eaten_at = $4, description = nullif($5, ''), updated_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING id, user_id, coalesce(meal_type, ''), eaten_at, coalesce(description, '')
	`, mealID, ownerID, in.MealType, in.EatenAt, in.Description).
		Scan(&m.ID, &m.UserID, &m.MealType, &m.EatenAt, &m.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return Meal{}, ErrNotFound
	}
	if err != nil {
		return Meal{}, fmt.Errorf("meal: update: %w", err)
	}

	photos, err := s.listPhotos(ctx, m.ID)
	if err != nil {
		return Meal{}, err
	}
	m.Photos = photos
	return m, nil
}

// softDelete keeps the row: photos stay addressable for the chat tombstone
// that slice 9 renders, and the reply thread around it is untouched.
func (s *store) softDelete(ctx context.Context, mealID, ownerID uuid.UUID, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE meal_records SET deleted_at = $3, updated_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, mealID, ownerID, at)
	if err != nil {
		return fmt.Errorf("meal: delete: %w", err)
	}
	return nil
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
	err := s.db.QueryRowContext(ctx, readableMealsCTE+`
		SELECT p.id, p.bucket, p.object_name, p.content_type
		FROM meal_photos p
		JOIN readable m ON m.id = p.meal_record_id
		WHERE p.id = $1
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
