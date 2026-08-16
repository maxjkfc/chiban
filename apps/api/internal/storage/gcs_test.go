package storage_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

// The rest of the suite runs against the in-memory ObjectStorage. This is the
// one test that proves the interface actually holds against fake-gcs-server,
// so a backend mismatch cannot hide behind the fake.
func TestGCSRoundTripAgainstFakeGCS(t *testing.T) {
	host := os.Getenv("CHIBAN_TEST_STORAGE_EMULATOR_HOST")
	if host == "" {
		t.Fatal("CHIBAN_TEST_STORAGE_EMULATOR_HOST is not set. Run tests with `make test`.")
	}

	ctx := t.Context()
	// A bucket of its own, so the smoke test never touches application data.
	const bucket = "chiban-storage-smoke-test"
	gcs, err := storage.NewGCS(ctx, host, []string{bucket})
	if err != nil {
		t.Fatalf("connect to fake-gcs: %v", err)
	}
	t.Cleanup(func() { gcs.Close() })

	const name = "smoke/hello.txt"
	const content = "hello chiban"

	stored, err := gcs.Upload(ctx, bucket, name, "text/plain", strings.NewReader(content))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if stored.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", stored.Size, len(content))
	}

	reader, object, err := gcs.Open(ctx, bucket, name)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
	if object.ContentType != "text/plain" {
		t.Fatalf("content type = %q, want %q", object.ContentType, "text/plain")
	}

	if err := gcs.Delete(ctx, bucket, name); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := gcs.Open(ctx, bucket, name); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("open after delete: err = %v, want ErrNotFound", err)
	}
}
