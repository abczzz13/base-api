package infraapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/abczzz13/base-api/internal/infraapi"
)

func TestDefaultReadinessCheckers(t *testing.T) {
	tests := []struct {
		name        string
		database    infraapi.DatabaseReadiness
		wantLen     int
		wantName    string
		wantCheckOK bool
		wantErrText []string
	}{
		{
			name:     "returns none when database is not configured",
			database: nil,
			wantLen:  0,
		},
		{
			name:        "adds database checker when database is configured",
			database:    &fakeDatabasePool{},
			wantLen:     1,
			wantName:    "database",
			wantCheckOK: true,
		},
		{
			name:     "propagates database readiness errors",
			database: &fakeDatabasePool{pingErr: errors.New("db unavailable")},
			wantLen:  1,
			wantName: "database",
			wantErrText: []string{
				"database readiness check failed",
				"db unavailable",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkers := infraapi.DefaultReadinessCheckers(tt.database)
			if got := len(checkers); got != tt.wantLen {
				t.Fatalf("checker length mismatch: want %d, got %d", tt.wantLen, got)
			}

			if len(checkers) == 0 {
				return
			}

			if tt.wantName != "" {
				if got := infraapi.ReadinessCheckerLogName(checkers[0], 0); got != tt.wantName {
					t.Fatalf("checker name mismatch: want %q, got %q", tt.wantName, got)
				}
			}

			err := checkers[0].CheckReadiness(context.Background())
			if tt.wantCheckOK && err != nil {
				t.Fatalf("CheckReadiness returned error: %v", err)
			}
			if !tt.wantCheckOK && err == nil {
				t.Fatal("CheckReadiness returned nil error")
			}
			for _, want := range tt.wantErrText {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("CheckReadiness error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}

func TestWithReadinessCheckerNameNilChecker(t *testing.T) {
	checker := infraapi.WithReadinessCheckerName("database", nil)
	if checker == nil {
		t.Fatal("withReadinessCheckerName returned nil checker")
	}

	err := checker.CheckReadiness(context.Background())
	if !errors.Is(err, infraapi.ErrNilReadinessChecker) {
		t.Fatalf("CheckReadiness error mismatch: want %v, got %v", infraapi.ErrNilReadinessChecker, err)
	}

	if got := infraapi.ReadinessCheckerLogName(checker, 0); got != "database" {
		t.Fatalf("checker name mismatch: want %q, got %q", "database", got)
	}
}

type fakeDatabasePool struct {
	pingErr error
}

func (p *fakeDatabasePool) Ping(context.Context) error {
	return p.pingErr
}
