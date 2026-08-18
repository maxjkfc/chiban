// Package meal owns MealRecord, the product's primary data.
//
// A meal exists on its own: chat only ever references it. This package is also
// the one place that writes to both PostgreSQL and object storage in a single
// operation, so the partial-failure handling here is what every later media
// feature reuses.
package meal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/media"
	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

var (
	// ErrNotFound covers both "no such meal" and "not yours", so the API never
	// confirms which meal IDs exist.
	ErrNotFound = errors.New("meal: not found")
	// ErrPhotoNotFound is the same idea for a single photo.
	ErrPhotoNotFound = errors.New("meal: photo not found")
	// ErrNotMember covers sharing into a group the owner does not belong to.
	ErrNotMember = errors.New("meal: not a member of that group")
)

// InvalidInputError describes input the caller can fix.
type InvalidInputError struct {
	Field   string
	Message string
}

func (e InvalidInputError) Error() string {
	return fmt.Sprintf("meal: invalid %s: %s", e.Field, e.Message)
}

const (
	// Bucket is where meal photos live. Clients never see this name.
	Bucket = "meal-images"

	MinPhotos = 1
	MaxPhotos = 4

	maxDescriptionLength = 500
)

// Types are the meal types V0.1 accepts. Meal type stays optional: making it
// required would slow down the photo-to-publish path that the product depends
// on.
var Types = []string{"breakfast", "lunch", "dinner", "snack", "other"}

type Meal struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	MealType    string
	EatenAt     time.Time
	Description string
	Photos      []Photo
}

type Photo struct {
	ID        uuid.UUID
	SortOrder int
}

// Input is a new or edited meal. Nothing about calories or nutrition appears
// here, deliberately: V0.1 is not a calorie tracker.
type Input struct {
	MealType    string
	EatenAt     time.Time
	Description string
}

// Upload is one file as it arrived from the client.
type Upload struct {
	Data []byte
}

// Membership answers which groups a reader belongs to. The group domain
// implements it; keeping it an interface here means meal never reads
// membership tables it does not own.
type Membership interface {
	GroupIDsFor(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

// Announcer posts a meal into a group's conversation. The chat domain
// implements it. Meal knows that sharing announces itself; it does not know
// what a chat message is made of.
type Announcer interface {
	AnnounceMeal(ctx context.Context, userID, groupID, mealID uuid.UUID) error
}

type Service struct {
	db        *sql.DB
	store     *store
	objects   storage.ObjectStorage
	members   Membership
	announcer Announcer
	now       func() time.Time
	newName   func(userID uuid.UUID, at time.Time, ext string) string
}

func NewService(db *sql.DB, objects storage.ObjectStorage, members Membership, announcer Announcer) *Service {
	return &Service{
		db:        db,
		store:     &store{db: db},
		objects:   objects,
		members:   members,
		announcer: announcer,
		now:       time.Now,
		newName:   objectName,
	}
}

// Create stores a meal and its photos.
//
// Object storage and PostgreSQL cannot share a transaction, so the order is
// deliberate: sanitise and upload everything first, then write all the
// metadata in one short transaction. If any step fails, the uploaded objects
// are deleted best-effort and nothing is left visible — a meal without its
// photos must never appear.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, in Input, uploads []Upload) (Meal, error) {
	if err := validate(&in); err != nil {
		return Meal{}, err
	}
	if len(uploads) < MinPhotos {
		return Meal{}, InvalidInputError{Field: "photos", Message: "at least one photo is required"}
	}
	if len(uploads) > MaxPhotos {
		return Meal{}, InvalidInputError{
			Field:   "photos",
			Message: fmt.Sprintf("at most %d photos are allowed", MaxPhotos),
		}
	}

	stored := make([]storage.Object, 0, len(uploads))
	// Anything already uploaded when a later step fails is an orphan; drop it.
	defer func() {
		if len(stored) == 0 {
			return
		}
		for _, object := range stored {
			// Best effort: the meal is already gone as far as the user is
			// concerned, and a leftover object is a cleanup-script problem,
			// not a reason to fail differently.
			_ = s.objects.Delete(context.WithoutCancel(ctx), object.Bucket, object.Name)
		}
	}()

	for _, upload := range uploads {
		sanitized, err := media.Sanitize(upload.Data)
		if err != nil {
			var invalid media.InvalidInputError
			if errors.As(err, &invalid) {
				return Meal{}, InvalidInputError{Field: "photos", Message: invalid.Message}
			}
			return Meal{}, err
		}

		object, err := s.objects.Upload(ctx, Bucket,
			s.newName(userID, s.now(), sanitized.Extension()),
			sanitized.ContentType, bytesReader(sanitized.Data))
		if err != nil {
			return Meal{}, fmt.Errorf("meal: upload photo: %w", err)
		}
		stored = append(stored, object)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Meal{}, fmt.Errorf("meal: begin: %w", err)
	}
	defer tx.Rollback()

	txStore := &store{db: tx}
	m, err := txStore.createMeal(ctx, userID, in)
	if err != nil {
		return Meal{}, err
	}
	for i, object := range stored {
		photo, err := txStore.createPhoto(ctx, m.ID, object, i)
		if err != nil {
			return Meal{}, err
		}
		m.Photos = append(m.Photos, photo)
	}
	if err := tx.Commit(); err != nil {
		return Meal{}, fmt.Errorf("meal: commit: %w", err)
	}

	// Committed: these objects belong to a real meal now, so the deferred
	// cleanup must leave them alone.
	stored = nil
	return m, nil
}

// UpdateInput is a partial edit. A nil field is left untouched.
type UpdateInput struct {
	MealType    *string
	EatenAt     *time.Time
	Description *string
}

// ListForDay returns the meals that belong to a calendar date for someone in
// loc, oldest first. The caller supplies the zone: which day a meal falls on
// is a property of the eater's profile, never of the server.
func (s *Service) ListForDay(ctx context.Context, userID uuid.UUID, date Date, loc *time.Location) ([]Meal, error) {
	start, end := DayRange(date, loc)
	return s.store.listBetween(ctx, userID, start, end)
}

// Update edits a meal. Only the owner may: this is not the same check as
// reading, which meal sharing widens later.
func (s *Service) Update(ctx context.Context, userID, mealID uuid.UUID, in UpdateInput) (Meal, error) {
	current, err := s.store.findOwned(ctx, mealID, userID)
	if err != nil {
		return Meal{}, err
	}

	next := Input{
		MealType:    current.MealType,
		EatenAt:     current.EatenAt,
		Description: current.Description,
	}
	if in.MealType != nil {
		next.MealType = *in.MealType
	}
	if in.EatenAt != nil {
		next.EatenAt = *in.EatenAt
	}
	if in.Description != nil {
		next.Description = *in.Description
	}
	if err := validate(&next); err != nil {
		return Meal{}, err
	}

	return s.store.update(ctx, mealID, userID, next)
}

// Delete soft-deletes a meal: it leaves the owner's history, but any chat
// thread that discussed it keeps its structure and shows a tombstone instead.
func (s *Service) Delete(ctx context.Context, userID, mealID uuid.UUID) error {
	if _, err := s.store.findOwned(ctx, mealID, userID); err != nil {
		return err
	}
	return s.store.softDelete(ctx, mealID, userID, s.now())
}

// Get returns a meal the requester is allowed to see. In this slice that means
// the owner; meal sharing widens it later, and every reader goes through here.
func (s *Service) Get(ctx context.Context, userID, mealID uuid.UUID) (Meal, error) {
	groupIDs, err := s.members.GroupIDsFor(ctx, userID)
	if err != nil {
		return Meal{}, err
	}
	return s.store.findForReader(ctx, mealID, userID, groupIDs)
}

// OpenPhoto streams a stored photo after checking the same authorization as
// reading the meal. The client addresses photos by ID; it never supplies a
// bucket or object name, so there is no path for it to traverse.
func (s *Service) OpenPhoto(ctx context.Context, userID, photoID uuid.UUID) (io.ReadCloser, string, error) {
	groupIDs, err := s.members.GroupIDsFor(ctx, userID)
	if err != nil {
		return nil, "", err
	}

	photo, err := s.store.findPhotoForReader(ctx, photoID, userID, groupIDs)
	if err != nil {
		return nil, "", err
	}

	reader, object, err := s.objects.Open(ctx, photo.Bucket, photo.ObjectName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, "", ErrPhotoNotFound
		}
		return nil, "", fmt.Errorf("meal: open photo: %w", err)
	}

	contentType := object.ContentType
	if contentType == "" {
		contentType = photo.ContentType
	}
	return reader, contentType, nil
}

func validate(in *Input) error {
	if in.EatenAt.IsZero() {
		return InvalidInputError{Field: "eaten_at", Message: "is required"}
	}
	in.Description = strings.TrimSpace(in.Description)
	if len([]rune(in.Description)) > maxDescriptionLength {
		return InvalidInputError{
			Field:   "description",
			Message: fmt.Sprintf("must be at most %d characters", maxDescriptionLength),
		}
	}
	if in.MealType == "" {
		return nil
	}
	for _, allowed := range Types {
		if in.MealType == allowed {
			return nil
		}
	}
	return InvalidInputError{Field: "meal_type", Message: "is not a valid meal type"}
}

// objectName keeps photos partitioned per user and month, which makes manual
// inspection and orphan cleanup on the Mac mini tractable.
func objectName(userID uuid.UUID, at time.Time, ext string) string {
	utc := at.UTC()
	return fmt.Sprintf("users/%s/meals/%04d/%02d/%s.%s",
		userID, utc.Year(), int(utc.Month()), uuid.NewString(), ext)
}

// Share makes a meal visible to groups the owner belongs to, and announces it
// in each one.
//
// The order matters: the share is what authorizes reading, so it is written
// first. A message that arrived before the share existed would show a card
// nobody could open.
func (s *Service) Share(ctx context.Context, ownerID, mealID uuid.UUID, groupIDs []uuid.UUID) error {
	// Only the owner decides who sees a meal.
	if _, err := s.store.findOwned(ctx, mealID, ownerID); err != nil {
		return err
	}

	for _, groupID := range groupIDs {
		// Sharing into a group you are not in would hand your meal to
		// strangers, so membership is checked per group rather than assumed.
		member, err := s.isMember(ctx, ownerID, groupID)
		if err != nil {
			return err
		}
		if !member {
			return ErrNotMember
		}

		if err := s.store.share(ctx, mealID, groupID); err != nil {
			return err
		}
		// Announcing is unconditional: posting the card is idempotent per meal
		// and group, so this is the one place that decides there is only ever
		// one card — including when a share is taken back and given again.
		if err := s.announcer.AnnounceMeal(ctx, ownerID, groupID, mealID); err != nil {
			return err
		}
	}
	return nil
}

// Unshare takes a meal back from a group. The meal message stays: the
// conversation it started belongs to the people who had it.
func (s *Service) Unshare(ctx context.Context, ownerID, mealID, groupID uuid.UUID) error {
	if _, err := s.store.findOwned(ctx, mealID, ownerID); err != nil {
		return err
	}
	_, err := s.store.revoke(ctx, mealID, groupID)
	return err
}

// SharedWith lists the groups a meal is currently shared with, for its owner.
func (s *Service) SharedWith(ctx context.Context, ownerID, mealID uuid.UUID) ([]uuid.UUID, error) {
	if _, err := s.store.findOwned(ctx, mealID, ownerID); err != nil {
		return nil, err
	}
	return s.store.sharedGroups(ctx, mealID)
}

func (s *Service) isMember(ctx context.Context, userID, groupID uuid.UUID) (bool, error) {
	groupIDs, err := s.members.GroupIDsFor(ctx, userID)
	if err != nil {
		return false, err
	}
	return slices.Contains(groupIDs, groupID), nil
}
