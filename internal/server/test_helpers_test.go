package server

import (
	"os"
	"strings"
	"testing"
)

func testDatabaseURL(tb testing.TB) string {
	tb.Helper()

	if value := strings.TrimSpace(os.Getenv("TEST_DB_URL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("DB_URL")); value != "" {
		return value
	}

	tb.Fatal("database-backed tests require TEST_DB_URL or DB_URL")

	return ""
}
