package outboundaudit

import (
	"context"
	"testing"
	"time"
)

func TestAsyncOutboundAuditStorePersistsRecordsThroughWrapper(t *testing.T) {
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
		Client:    "billing",
		Operation: "get_invoice",
		Method:    "GET",
		Host:      "billing.example",
		Path:      "/invoices/123",
	}
	if err := store.StoreOutboundAudit(context.Background(), record); err != nil {
		t.Fatalf("StoreOutboundAudit returned error: %v", err)
	}

	shutdown()

	if len(stored) != 1 {
		t.Fatalf("stored count mismatch: want 1, got %d", len(stored))
	}
	if stored[0].Path != "/invoices/123" {
		t.Fatalf("path mismatch: want %q, got %q", "/invoices/123", stored[0].Path)
	}
}

func TestNewAsyncRepositoryReturnsNopForNilRepository(t *testing.T) {
	store, shutdown := NewAsyncRepository(nil, AsyncConfig{})
	defer shutdown()

	if err := store.StoreOutboundAudit(context.Background(), Record{}); err != nil {
		t.Fatalf("StoreOutboundAudit on nil repo returned error: %v", err)
	}
}
