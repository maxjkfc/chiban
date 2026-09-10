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

type Membership interface {
	IsMember(ctx context.Context, userID, groupID uuid.UUID) (bool, error)
	GroupIDsFor(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

type PushNotifier interface {
	NotifyMealShare(ctx context.Context, groupIDs []uuid.UUID, senderID uuid.UUID, mealID uuid.UUID)
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
	push      PushNotifier
	now       func() time.Time
	newName   func(userID uuid.UUID, at time.Time, ext string) string
}

func (s *Service) SetPushNotifier(p PushNotifier) {
	s.push = p
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

// MaxDailySummaryRangeDays bounds how many calendar days DailySummary will
// answer for in one call — about a quarter. Trophy rows and month calendars
// are the only two shapes that call it, and neither needs more than that;
// letting the range grow without a limit would turn one request into an
// unbounded table scan of a user's whole history.
const MaxDailySummaryRangeDays = 92

// DailySummaryDay is one calendar day's worth of recording activity for its
// owner: how many meals were logged and what types they were. It always
// exists for every date in the requested range, even a day with zero meals,
// so a client can render a full calendar grid or streak row without having
// to fill in the gaps itself.
type DailySummaryDay struct {
	Date Date
	// MealCount is every meal recorded that day, including duplicates of the
	// same meal type — a second breakfast is two, not one.
	MealCount int
	// MealTypes is one entry per meal in eaten_at order, "" for a meal saved
	// without a type. len(MealTypes) always equals MealCount: this is the
	// per-meal detail behind the count, not a deduplicated badge set, so a
	// client can tell "three separate lunches" apart from "breakfast, lunch,
	// dinner" without a second request.
	MealTypes []string
}

// DailySummary aggregates a user's own meal_records into one entry per
// calendar day in [start, end], inclusive on both ends, bucketed by that
// user's own timezone — never the server's local time or UTC. It is the one
// query behind both the 7-day trophy row and the month calendar view: both
// read this same shape and never need a second query to fill in a day.
func (s *Service) DailySummary(ctx context.Context, userID uuid.UUID, start, end Date, loc *time.Location) ([]DailySummaryDay, error) {
	span := start.DaysUntil(end)
	if span < 0 {
		return nil, InvalidInputError{Field: "end", Message: "must not be before start"}
	}
	if span+1 > MaxDailySummaryRangeDays {
		return nil, InvalidInputError{
			Field:   "end",
			Message: fmt.Sprintf("the range must not exceed %d days", MaxDailySummaryRangeDays),
		}
	}

	// One instant range covers the whole request: DayRange is monotonic in
	// its date argument, so the first day's start and the last day's end
	// bound every day in between.
	rangeStart, _ := DayRange(start, loc)
	_, rangeEnd := DayRange(end, loc)

	rows, err := s.store.eatenBetween(ctx, userID, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}

	byDate := map[Date]*DailySummaryDay{}
	for _, row := range rows {
		date := DateOf(row.EatenAt, loc)
		day, ok := byDate[date]
		if !ok {
			day = &DailySummaryDay{Date: date}
			byDate[date] = day
		}
		day.MealCount++
		day.MealTypes = append(day.MealTypes, row.MealType)
	}

	out := make([]DailySummaryDay, 0, span+1)
	for d := start; ; d = d.AddDays(1) {
		if day, ok := byDate[d]; ok {
			out = append(out, *day)
		} else {
			out = append(out, DailySummaryDay{Date: d, MealTypes: []string{}})
		}
		if d == end {
			break
		}
	}
	return out, nil
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

// Get returns a meal the requester is allowed to see: its owner, or a member
// of a group it is currently shared with. Every reader goes through here.
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

	// One lookup for the whole request, and every group checked before
	// anything is written. Sharing into a group you are not in would hand your
	// meal to strangers, and refusing halfway would leave the earlier groups
	// already shared while the caller is told the request failed.
	mine, err := s.members.GroupIDsFor(ctx, ownerID)
	if err != nil {
		return err
	}
	for _, groupID := range groupIDs {
		if !slices.Contains(mine, groupID) {
			return ErrNotMember
		}
	}

	for _, groupID := range groupIDs {
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
	if s.push != nil && len(groupIDs) > 0 {
		s.push.NotifyMealShare(ctx, groupIDs, ownerID, mealID)
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
