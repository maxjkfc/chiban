package storage

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// Memory is an in-memory ObjectStorage used by integration tests, so the test
// suite exercises real handler and service code without depending on a running
// fake-gcs-server.
type Memory struct {
	mu      sync.Mutex
	objects map[string]memoryObject

	// FailUpload, when set, makes Upload return this error. Tests use it to
	// exercise the storage/DB partial-failure cleanup path.
	FailUpload error
	// FailUploadAfter, when positive, lets that many uploads succeed before
	// FailUpload starts applying, so a test can fail the third photo of four
	// and check the first two were cleaned up.
	FailUploadAfter int

	uploads int
}

type memoryObject struct {
	data        []byte
	contentType string
}

func NewMemory() *Memory {
	return &Memory{objects: map[string]memoryObject{}}
}

func (m *Memory) Upload(_ context.Context, bucket, name, contentType string, r io.Reader) (Object, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Object{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.FailUpload != nil && m.uploads >= m.FailUploadAfter {
		return Object{}, m.FailUpload
	}
	m.uploads++

	m.objects[key(bucket, name)] = memoryObject{data: data, contentType: contentType}
	return Object{Bucket: bucket, Name: name, ContentType: contentType, Size: int64(len(data))}, nil
}

func (m *Memory) Open(_ context.Context, bucket, name string) (io.ReadCloser, Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[key(bucket, name)]
	if !ok {
		return nil, Object{}, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(o.data)), Object{
		Bucket:      bucket,
		Name:        name,
		ContentType: o.contentType,
		Size:        int64(len(o.data)),
	}, nil
}

func (m *Memory) Delete(_ context.Context, bucket, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[key(bucket, name)]; !ok {
		return ErrNotFound
	}
	delete(m.objects, key(bucket, name))
	return nil
}

// Len reports how many objects are stored, so tests can assert that a failed
// meal upload left nothing behind.
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.objects)
}

func key(bucket, name string) string { return bucket + "/" + name }
