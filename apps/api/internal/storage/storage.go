// Package storage abstracts object storage behind a small interface.
//
// The rest of the application never learns which backend is in use, and
// never exposes bucket names or object names to clients: media is always
// addressed by an application-level ID and streamed through the API.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound is returned by Open and Delete when the object does not exist.
var ErrNotFound = errors.New("storage: object not found")

// Object is the metadata the application cares about. Anything backend
// specific stays inside the implementation.
type Object struct {
	Bucket      string
	Name        string
	ContentType string
	Size        int64
}

type ObjectStorage interface {
	// Upload stores r under bucket/name and returns the stored object.
	Upload(ctx context.Context, bucket, name, contentType string, r io.Reader) (Object, error)
	// Open returns a reader for the object. The caller must close it.
	Open(ctx context.Context, bucket, name string) (io.ReadCloser, Object, error)
	// Delete removes the object. Deleting a missing object returns ErrNotFound.
	Delete(ctx context.Context, bucket, name string) error
}
