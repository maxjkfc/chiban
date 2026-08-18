package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/media"
	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

// Bucket is where chat media lives. One bucket per kind of thing keeps the
// storage layout readable without leaking it to any client.
const Bucket = "chat-media"

// Media is an uploaded image or GIF, addressed by application id. Buckets and
// object names never leave this package.
type Media struct {
	ID   uuid.UUID
	Type string
}

// UploadMedia validates and stores one image or GIF.
//
// It goes through the same media pipeline as every other upload in the
// product, on the branch that keeps animation: a GIF that arrived animated has
// to still be animated when it comes back, which re-encoding would destroy.
func (s *Service) UploadMedia(ctx context.Context, userID uuid.UUID, data []byte) (Media, error) {
	sanitized, err := media.SanitizeAllowingGIF(data)
	if err != nil {
		var invalid media.InvalidInputError
		if errors.As(err, &invalid) {
			return Media{}, InvalidInputError{Field: "file", Message: invalid.Message}
		}
		return Media{}, fmt.Errorf("chat: sanitize media: %w", err)
	}

	mediaType := TypeImage
	if sanitized.Kind == media.KindGIF {
		mediaType = TypeGIF
	}

	object, err := s.objects.Upload(ctx, Bucket,
		mediaObjectName(userID), sanitized.ContentType, bytes.NewReader(sanitized.Data))
	if err != nil {
		return Media{}, fmt.Errorf("chat: upload media: %w", err)
	}

	id, err := s.store.insertMedia(ctx, userID, mediaType, object, len(sanitized.Data))
	if err != nil {
		// The row is what makes the object reachable, so an object with no row
		// is unreachable rubbish. Detached from the request context: the write
		// most likely failed because the request was cancelled, and a cleanup
		// on that same context would fail for the same reason — exactly the
		// case this exists for.
		_ = s.objects.Delete(context.WithoutCancel(ctx), object.Bucket, object.Name)
		return Media{}, err
	}
	return Media{ID: id, Type: mediaType}, nil
}

// OpenMedia streams media the reader is allowed to see.
//
// Allowed means: they uploaded it, or it is attached to a live message in a
// group they belong to. An upload that has not been posted anywhere is visible
// only to the person who made it, and posting it is what widens that.
func (s *Service) OpenMedia(ctx context.Context, readerID, mediaID uuid.UUID) (io.ReadCloser, string, error) {
	stored, err := s.store.findMediaForReader(ctx, mediaID, readerID)
	if err != nil {
		return nil, "", err
	}

	reader, object, err := s.objects.Open(ctx, stored.Bucket, stored.ObjectName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("chat: open media: %w", err)
	}

	contentType := object.ContentType
	if contentType == "" {
		contentType = stored.ContentType
	}
	return reader, contentType, nil
}

func mediaObjectName(userID uuid.UUID) string {
	return "chat-media/" + userID.String() + "/" + uuid.NewString()
}
