package postgres

import (
	"context"
	"testing"
)

func TestMigrateValidation(t *testing.T) {
	err := Migrate(context.Background(), nil)
	if err == nil {
		t.Fatal("Migrate returned nil error")
	}
	if got, want := err.Error(), "database pool is required"; got != want {
		t.Fatalf("error mismatch: want %q, got %q", want, got)
	}
}
