// Package sticker holds a user's own reusable images and GIFs.
//
// A sticker is personal property: its owner uploads it, lists it, sends it and
// deletes it. Everything about who may look at one follows from that, so this
// package owns the table and nothing else reads it.
package sticker

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/media"
	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

// Bucket is where stickers live. Clients never see this name, but it has to
// match the one the process provisions at startup — see the test that pins
// every domain's bucket to config.Buckets.
const Bucket = "user-stickers"

// MaxPerUser bounds one person's library. A picker is only useful while you
// can still find things in it, and an unbounded library is an unbounded upload
// budget per account.
const MaxPerUser = 200

var (
	// ErrNotFound covers "no such sticker" and "not yours" alike. Telling the
	// two apart would confirm which sticker ids are real.
	ErrNotFound = errors.New("sticker: not found")
	// ErrTooMany is returned once a library is full.
	ErrTooMany = errors.New("sticker: too many stickers")
)

// InvalidInputError is a rejection the sender can fix.
type InvalidInputError struct {
	Field   string
	Message string
}

func (e InvalidInputError) Error() string { return e.Field + " " + e.Message }

// Viewership answers whether two people are in a group together. The group
// domain implements it; keeping it an interface here means this package never
// reads membership tables it does not own.
//
// It is the same question avatars ask, for the same reason: a sticker is a
// personal picture that reaches other people by appearing in the groups its
// owner is in.
type Viewership interface {
	SharesGroup(ctx context.Context, a, b uuid.UUID) (bool, error)
}

// Sticker is one entry in a library, addressed by application id.
type Sticker struct {
	ID   uuid.UUID
	Type string
}

type Service struct {
	store   *store
	objects storage.ObjectStorage
	viewers Viewership
}

func NewService(db *sql.DB, objects storage.ObjectStorage, viewers Viewership) *Service {
	return &Service{store: &store{db: db}, objects: objects, viewers: viewers}
}

// Add validates and stores one sticker.
//
// It goes through the same media pipeline as every other upload in the
// product, on the branch that keeps animation: a GIF sticker that arrived
// animated has to still be animated when it comes back.
func (s *Service) Add(ctx context.Context, userID uuid.UUID, data []byte) (Sticker, error) {
	count, err := s.store.countForOwner(ctx, userID)
	if err != nil {
		return Sticker{}, err
	}
	if count >= MaxPerUser {
		return Sticker{}, ErrTooMany
	}

	sanitized, err := media.SanitizeAllowingGIF(data)
	if err != nil {
		var invalid media.InvalidInputError
		if errors.As(err, &invalid) {
			return Sticker{}, InvalidInputError{Field: "file", Message: invalid.Message}
		}
		return Sticker{}, fmt.Errorf("sticker: sanitize: %w", err)
	}

	stickerType := TypeImage
	if sanitized.Kind == media.KindGIF {
		stickerType = TypeGIF
	}

	object, err := s.objects.Upload(ctx, Bucket,
		objectName(userID), sanitized.ContentType, bytes.NewReader(sanitized.Data))
	if err != nil {
		return Sticker{}, fmt.Errorf("sticker: upload: %w", err)
	}

	id, err := s.store.insert(ctx, userID, stickerType, object, len(sanitized.Data))
	if err != nil {
		// The row is what makes the object reachable, so an object with no row
		// is unreachable rubbish. Detached from the request context: the write
		// most likely failed because the request was cancelled, and a cleanup
		// on that same context would fail for the same reason — exactly the
		// case this exists for.
		_ = s.objects.Delete(context.WithoutCancel(ctx), object.Bucket, object.Name)
		return Sticker{}, err
	}
	return Sticker{ID: id, Type: stickerType}, nil
}

// List returns the owner's live stickers, newest first.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Sticker, error) {
	return s.store.listForOwner(ctx, userID)
}

// Remove takes a sticker out of its owner's library.
//
// Only the owner can, and it is a soft delete: messages already sent with this
// sticker keep rendering it. Deleting is for tidying the picker, not for
// retracting what was already said.
func (s *Service) Remove(ctx context.Context, userID, stickerID uuid.UUID) error {
	return s.store.softDelete(ctx, stickerID, userID)
}

// BelongsTo reports whether a sticker is this user's to send. Chat calls it
// through its own interface before posting one.
//
// A deleted sticker cannot be sent again: it is gone from the picker, and
// letting it back in through the API would make deletion meaningless.
func (s *Service) BelongsTo(ctx context.Context, stickerID, userID uuid.UUID) (bool, error) {
	return s.store.isLiveOwner(ctx, stickerID, userID)
}

// Open streams a sticker the reader is allowed to see.
//
// Allowed means: it is theirs, or they are in a group with its owner. Deleted
// stickers are still served — messages sent before the deletion have to keep
// working, which is the whole reason deletion is soft.
func (s *Service) Open(ctx context.Context, readerID, stickerID uuid.UUID) (io.ReadCloser, string, error) {
	stored, err := s.store.find(ctx, stickerID)
	if err != nil {
		return nil, "", err
	}

	if stored.OwnerID != readerID {
		shared, err := s.viewers.SharesGroup(ctx, readerID, stored.OwnerID)
		if err != nil {
			return nil, "", fmt.Errorf("sticker: check viewership: %w", err)
		}
		if !shared {
			return nil, "", ErrNotFound
		}
	}

	reader, object, err := s.objects.Open(ctx, stored.Bucket, stored.ObjectName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("sticker: open: %w", err)
	}

	contentType := object.ContentType
	if contentType == "" {
		contentType = stored.ContentType
	}
	return reader, contentType, nil
}

// The two kinds a sticker can be. The kind comes from what was decoded, never
// from what the uploader claimed.
const (
	TypeImage = "image"
	TypeGIF   = "gif"
)

func objectName(userID uuid.UUID) string {
	return "stickers/" + userID.String() + "/" + uuid.NewString()
}
