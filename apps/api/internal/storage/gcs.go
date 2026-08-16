package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// GCS talks to a GCS-compatible endpoint. In V0.1 that endpoint is always
// fake-gcs-server; the real Google Cloud Storage is never contacted.
type GCS struct {
	client *storage.Client
}

// NewGCS connects to the emulator at emulatorHost (e.g. "http://fake-gcs:4443")
// and makes sure every bucket exists.
func NewGCS(ctx context.Context, emulatorHost string, buckets []string) (*GCS, error) {
	if emulatorHost == "" {
		return nil, errors.New("storage: emulator host is required")
	}
	// The GCS client reads STORAGE_EMULATOR_HOST to redirect all traffic and to
	// skip credential lookup. Setting it here keeps the caller from having to.
	if err := os.Setenv("STORAGE_EMULATOR_HOST", emulatorHost); err != nil {
		return nil, fmt.Errorf("storage: set emulator host: %w", err)
	}

	// WithJSONReads keeps object reads on the JSON API. The client's default
	// XML/path-style reads (GET {host}/{bucket}/{object}) are only served by
	// fake-gcs-server when the request Host matches its -public-host, which
	// cannot be true for both the API container and host-run tests.
	client, err := storage.NewClient(ctx, option.WithoutAuthentication(), storage.WithJSONReads())
	if err != nil {
		return nil, fmt.Errorf("storage: new client: %w", err)
	}

	g := &GCS{client: client}
	for _, bucket := range buckets {
		if err := g.ensureBucket(ctx, bucket); err != nil {
			client.Close()
			return nil, err
		}
	}
	return g, nil
}

func (g *GCS) Close() error { return g.client.Close() }

func (g *GCS) ensureBucket(ctx context.Context, bucket string) error {
	b := g.client.Bucket(bucket)
	if _, err := b.Attrs(ctx); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrBucketNotExist) {
		return fmt.Errorf("storage: inspect bucket %s: %w", bucket, err)
	}
	// projectID is ignored by the emulator but required by the API signature.
	if err := b.Create(ctx, "chiban-local", nil); err != nil {
		return fmt.Errorf("storage: create bucket %s: %w", bucket, err)
	}
	return nil
}

func (g *GCS) Upload(ctx context.Context, bucket, name, contentType string, r io.Reader) (Object, error) {
	w := g.client.Bucket(bucket).Object(name).NewWriter(ctx)
	w.ContentType = contentType
	size, err := io.Copy(w, r)
	if err != nil {
		_ = w.Close()
		return Object{}, fmt.Errorf("storage: write object: %w", err)
	}
	if err := w.Close(); err != nil {
		return Object{}, fmt.Errorf("storage: finalize object: %w", err)
	}
	return Object{Bucket: bucket, Name: name, ContentType: contentType, Size: size}, nil
}

func (g *GCS) Open(ctx context.Context, bucket, name string) (io.ReadCloser, Object, error) {
	obj := g.client.Bucket(bucket).Object(name)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, Object{}, ErrNotFound
		}
		return nil, Object{}, fmt.Errorf("storage: object attrs: %w", err)
	}
	r, err := obj.NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, Object{}, ErrNotFound
		}
		return nil, Object{}, fmt.Errorf("storage: read object: %w", err)
	}
	return r, Object{
		Bucket:      bucket,
		Name:        name,
		ContentType: attrs.ContentType,
		Size:        attrs.Size,
	}, nil
}

func (g *GCS) Delete(ctx context.Context, bucket, name string) error {
	err := g.client.Bucket(bucket).Object(name).Delete(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("storage: delete object: %w", err)
	}
	return nil
}
