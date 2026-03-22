package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/weatheroas"
)

func TestKnownPublicOperationNamesMatchesGeneratedOperations(t *testing.T) {
	wantOperations := map[string]struct{}{
		publicoas.CreateNoteOperation:         {},
		publicoas.DeleteNoteOperation:         {},
		publicoas.GetHealthzOperation:         {},
		publicoas.GetNoteOperation:            {},
		publicoas.ListNotesOperation:          {},
		publicoas.UpdateNoteOperation:         {},
		weatheroas.GetCurrentWeatherOperation: {},
	}

	if diff := cmp.Diff(wantOperations, knownPublicOperationNames); diff != "" {
		t.Fatalf("knownPublicOperationNames drift (-want +got):\n%s", diff)
	}
}
