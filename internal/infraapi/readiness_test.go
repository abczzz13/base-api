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
		name       string
		database   infraapi.DatabaseReadiness
		valkey     infraapi.ValkeyReadiness
		wantLen    int
		wantNames  []string
		wantErrors []string
	}{
		{
			name:    "returns none when no dependencies configured",
			wantLen: 0,
		},
		{
			name:      "adds database checker when database is configured",
			database:  &fakeDatabasePool{},
			wantLen:   1,
			wantNames: []string{"database"},
		},
		{
			name:      "adds valkey checker when valkey is configured",
			valkey:    &fakeValkeyPinger{},
			wantLen:   1,
			wantNames: []string{"valkey"},
		},
		{
			name:      "adds both checkers when both configured",
			database:  &fakeDatabasePool{},
			valkey:    &fakeValkeyPinger{},
			wantLen:   2,
			wantNames: []string{"database", "valkey"},
		},
		{
			name:       "propagates database readiness errors",
			database:   &fakeDatabasePool{pingErr: errors.New("db unavailable")},
			wantLen:    1,
			wantNames:  []string{"database"},
			wantErrors: []string{"db unavailable"},
		},
		{
			name:       "propagates valkey readiness errors",
			valkey:     &fakeValkeyPinger{pingErr: errors.New("valkey unavailable")},
			wantLen:    1,
			wantNames:  []string{"valkey"},
			wantErrors: []string{"valkey unavailable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkers := infraapi.DefaultReadinessCheckers(tt.database, tt.valkey)
			if got := len(checkers); got != tt.wantLen {
				t.Fatalf("checker length mismatch: want %d, got %d", tt.wantLen, got)
			}

			for i, wantName := range tt.wantNames {
				if got := infraapi.ReadinessCheckerLogName(checkers[i], i); got != wantName {
					t.Fatalf("checker[%d] name mismatch: want %q, got %q", i, wantName, got)
				}
			}

			for i, checker := range checkers {
				err := checker.CheckReadiness(context.Background())
				if i < len(tt.wantErrors) && tt.wantErrors[i] != "" {
					if err == nil {
						t.Fatalf("checker[%d] returned nil error", i)
					}
					if !strings.Contains(err.Error(), tt.wantErrors[i]) {
						t.Fatalf("checker[%d] error %q does not contain %q", i, err.Error(), tt.wantErrors[i])
					}
				} else if err != nil {
					t.Fatalf("checker[%d] returned unexpected error: %v", i, err)
				}
			}
		})
	}
}

type fakeDatabasePool struct {
	pingErr error
}

func (p *fakeDatabasePool) Ping(context.Context) error {
	return p.pingErr
}

type fakeValkeyPinger struct {
	pingErr error
}

func (p *fakeValkeyPinger) Ping(context.Context) error {
	return p.pingErr
}
