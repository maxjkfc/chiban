// Package profile owns the small amount of identity a group sees: display
// name and the timezone that decides where the user's day starts and ends.
//
// V0.1 deliberately stores nothing else. Height, weight, sex and calorie
// targets belong to a later version and are not collected here.
package profile

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	// Embeds the IANA timezone database in the binary. The API runs on a
	// distroless image with no system tzdata, so without this every
	// LoadLocation call would fail there but succeed on a developer's Mac.
	_ "time/tzdata"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/media"
	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

var (
	// ErrNotFound means the user has not completed onboarding yet.
	ErrNotFound = errors.New("profile: not found")
	// ErrAvatarNotFound covers an unknown or already-replaced media ID.
	ErrAvatarNotFound = errors.New("profile: avatar not found")
)

// InvalidInputError describes input the caller can fix.
type InvalidInputError struct {
	Field   string
	Message string
}

func (e InvalidInputError) Error() string {
	return fmt.Sprintf("profile: invalid %s: %s", e.Field, e.Message)
}

const maxDisplayNameLength = 50

type Profile struct {
	UserID      uuid.UUID
	DisplayName string
	Timezone    string
	// AvatarMediaID is what the client uses to fetch the picture. It is a
	// fresh UUID on every upload, so it doubles as a cache key: a changed
	// avatar is a different URL rather than a stale one.
	AvatarMediaID uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// HasAvatar reports whether a picture has been uploaded.
func (p Profile) HasAvatar() bool { return p.AvatarMediaID != uuid.Nil }

// Input carries a partial update. A nil field is left untouched, which is what
// makes PATCH semantics work; both are required when the profile is created.
type Input struct {
	DisplayName *string
	Timezone    *string
}

// Bucket is where avatars live. Clients never see this name.
const Bucket = "avatars"

// Viewership answers whether one user may see another's picture. The group
// domain implements it; keeping it an interface here means this package never
// reads membership tables it does not own.
type Viewership interface {
	SharesGroup(ctx context.Context, a, b uuid.UUID) (bool, error)
}

type Service struct {
	db      *sql.DB
	store   *store
	objects storage.ObjectStorage
	viewers Viewership
	newName func(userID uuid.UUID, ext string) string
}

func NewService(db *sql.DB, objects storage.ObjectStorage, viewers Viewership) *Service {
	return &Service{
		db:      db,
		store:   &store{db: db},
		objects: objects,
		viewers: viewers,
		newName: objectName,
	}
}

// SaveAvatar stores a new picture and points the profile at it.
//
// The old object is deleted only after the new one is committed: losing the
// previous picture because the replacement failed would be worse than leaving
// one object behind for the cleanup script.
func (s *Service) SaveAvatar(ctx context.Context, userID uuid.UUID, data []byte) (Profile, error) {
	// Same pipeline as meal photos, so an avatar is validated, size-limited
	// and stripped of EXIF/GPS on exactly the same terms.
	sanitized, err := media.Sanitize(data)
	if err != nil {
		var invalid media.InvalidInputError
		if errors.As(err, &invalid) {
			return Profile{}, InvalidInputError{Field: "avatar", Message: invalid.Message}
		}
		return Profile{}, err
	}

	if _, err := s.store.find(ctx, userID); err != nil {
		// No profile means onboarding is unfinished; there is nothing to
		// attach a picture to yet.
		return Profile{}, err
	}

	// Upload before the transaction: network I/O inside one would hold the row
	// lock for as long as storage takes to answer.
	object, err := s.objects.Upload(ctx, Bucket,
		s.newName(userID, sanitized.Extension()), sanitized.ContentType, bytes.NewReader(sanitized.Data))
	if err != nil {
		return Profile{}, fmt.Errorf("profile: upload avatar: %w", err)
	}

	updated, previous, err := s.swapAvatar(ctx, userID, object)
	if err != nil {
		// The upload succeeded but the pointer did not: drop the orphan.
		_ = s.objects.Delete(context.WithoutCancel(ctx), object.Bucket, object.Name)
		return Profile{}, err
	}

	if previous.ObjectName != "" {
		// Best effort: the profile already points at the new picture, so a
		// leftover object is a cleanup-script problem, not a failed upload.
		_ = s.objects.Delete(context.WithoutCancel(ctx), previous.Bucket, previous.ObjectName)
	}
	return updated, nil
}

// swapAvatar points the profile at a new object and reports which one it
// replaced.
//
// Reading the outgoing location and moving the pointer happen under one row
// lock. Two uploads racing each other would otherwise both see the same
// previous object, each delete only that one, and leave the loser's upload
// unreferenced in storage forever. The lock also means the old location is
// still readable: once the pointer has moved, nothing in the row remembers it.
func (s *Service) swapAvatar(ctx context.Context, userID uuid.UUID, object storage.Object) (Profile, storedAvatar, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, storedAvatar{}, fmt.Errorf("profile: begin: %w", err)
	}
	defer tx.Rollback()

	txStore := &store{db: tx}
	previous, err := txStore.lockAvatarForUser(ctx, userID)
	if err != nil {
		return Profile{}, storedAvatar{}, err
	}

	updated, err := txStore.setAvatar(ctx, userID, uuid.New(), object)
	if err != nil {
		return Profile{}, storedAvatar{}, err
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, storedAvatar{}, fmt.Errorf("profile: commit: %w", err)
	}
	return updated, previous, nil
}

// OpenAvatar streams a stored avatar to someone allowed to see it: its owner,
// or a user currently in a group with them.
//
// An unguessable ID is not the check. Every other media read in this codebase
// re-verifies a live relationship on each request, and MVP_SPEC §6.1 scopes a
// picture to the groups you are in — so leaving a group has to stop working
// immediately, and an ID that leaks by any other route must be useless.
func (s *Service) OpenAvatar(ctx context.Context, requesterID, mediaID uuid.UUID) (io.ReadCloser, string, error) {
	stored, err := s.store.findAvatarObject(ctx, mediaID)
	if err != nil {
		return nil, "", err
	}

	allowed, err := s.viewers.SharesGroup(ctx, requesterID, stored.OwnerID)
	if err != nil {
		return nil, "", err
	}
	if !allowed {
		// Same answer as an unknown ID: whether a picture exists is not
		// something a stranger gets to learn.
		return nil, "", ErrAvatarNotFound
	}

	reader, object, err := s.objects.Open(ctx, stored.Bucket, stored.ObjectName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, "", ErrAvatarNotFound
		}
		return nil, "", fmt.Errorf("profile: open avatar: %w", err)
	}

	contentType := object.ContentType
	if contentType == "" {
		contentType = stored.ContentType
	}
	return reader, contentType, nil
}

// objectName keeps one avatar path per user; the UUID makes replacement a new
// object rather than an overwrite, so a cached URL never shows the wrong face.
func objectName(userID uuid.UUID, ext string) string {
	return fmt.Sprintf("users/%s/avatar/%s.%s", userID, uuid.NewString(), ext)
}

func (s *Service) Get(ctx context.Context, userID uuid.UUID) (Profile, error) {
	return s.store.find(ctx, userID)
}

// Save creates the profile on first call and updates it afterwards, so
// onboarding and later edits are the same operation.
func (s *Service) Save(ctx context.Context, userID uuid.UUID, in Input) (Profile, error) {
	current, err := s.store.find(ctx, userID)
	creating := errors.Is(err, ErrNotFound)
	if err != nil && !creating {
		return Profile{}, err
	}

	displayName := current.DisplayName
	if in.DisplayName != nil {
		displayName = strings.TrimSpace(*in.DisplayName)
	}
	timezone := current.Timezone
	if in.Timezone != nil {
		timezone = strings.TrimSpace(*in.Timezone)
	}

	if err := validateDisplayName(displayName); err != nil {
		return Profile{}, err
	}
	if err := validateTimezone(timezone); err != nil {
		return Profile{}, err
	}

	return s.store.upsert(ctx, userID, displayName, timezone)
}

func validateDisplayName(name string) error {
	if name == "" {
		return InvalidInputError{Field: "display_name", Message: "is required"}
	}
	if len([]rune(name)) > maxDisplayNameLength {
		return InvalidInputError{
			Field:   "display_name",
			Message: fmt.Sprintf("must be at most %d characters", maxDisplayNameLength),
		}
	}
	return nil
}

// validateTimezone insists on a real IANA name. Storing an unloadable zone
// would break every Today and history query for that user later on.
func validateTimezone(name string) error {
	if name == "" {
		return InvalidInputError{Field: "timezone", Message: "is required"}
	}
	// LoadLocation accepts "Local", but that would make the user's day
	// boundary depend on whichever machine serves the request. UTC is fine.
	if name == "Local" {
		return InvalidInputError{Field: "timezone", Message: "must be a specific IANA timezone"}
	}
	if _, err := time.LoadLocation(name); err != nil {
		return InvalidInputError{Field: "timezone", Message: "is not a valid IANA timezone"}
	}
	return nil
}

// Summary is the part of a profile other members see.
type Summary struct {
	DisplayName   string
	AvatarMediaID uuid.UUID
}

// HasAvatar reports whether a picture has been uploaded.
func (s Summary) HasAvatar() bool { return s.AvatarMediaID != uuid.Nil }

// Summaries resolves several users at once, so callers listing a group's
// members do not need a query per member. Users without a profile are simply
// absent from the map.
func (s *Service) Summaries(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]Summary, error) {
	return s.store.summaries(ctx, userIDs)
}
