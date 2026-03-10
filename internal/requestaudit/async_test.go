package requestaudit

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestAsyncRequestAuditStorePersistsRecordsThroughWrapper(t *testing.T) {
	t.Parallel()

	var stored []Record
	delegate := RepositoryFunc(func(_ context.Context, record Record) error {
		stored = append(stored, record)
		return nil
	})

	store, shutdown := NewAsyncRepository(delegate, AsyncConfig{
		QueueSize:    2,
		WriteTimeout: time.Second,
	})
	defer shutdown()

	record := Record{
		Server:     "public",
		Method:     http.MethodGet,
		Path:       "/healthz",
		StatusCode: http.StatusOK,
	}
	if err := store.StoreRequestAudit(context.Background(), record); err != nil {
		t.Fatalf("StoreRequestAudit returned error: %v", err)
	}

	shutdown()

	if len(stored) != 1 {
		t.Fatalf("stored count mismatch: want 1, got %d", len(stored))
	}
	if stored[0].Path != "/healthz" {
		t.Fatalf("path mismatch: want %q, got %q", "/healthz", stored[0].Path)
	}
}

func TestNewAsyncRepositoryReturnsNopForNilRepository(t *testing.T) {
	store, shutdown := NewAsyncRepository(nil, AsyncConfig{})
	defer shutdown()

	if err := store.StoreRequestAudit(context.Background(), Record{}); err != nil {
		t.Fatalf("StoreRequestAudit on nil repo returned error: %v", err)
	}
}
