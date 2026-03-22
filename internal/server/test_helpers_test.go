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

	tb.Fatal("set TEST_DB_URL to run database-backed tests")

	return ""
}
