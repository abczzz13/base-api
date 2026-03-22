package config

import (
	"slices"
	"strings"

	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/weatheroas"
)

var knownPublicOperationNames = map[string]struct{}{
	publicoas.CreateNoteOperation:         {},
	publicoas.DeleteNoteOperation:         {},
	publicoas.GetHealthzOperation:         {},
	publicoas.GetNoteOperation:            {},
	publicoas.ListNotesOperation:          {},
	publicoas.UpdateNoteOperation:         {},
	weatheroas.GetCurrentWeatherOperation: {},
}

func isKnownPublicOperationName(operationName string) bool {
	_, ok := knownPublicOperationNames[strings.TrimSpace(operationName)]
	return ok
}

func knownPublicOperationNamesList() []string {
	names := make([]string, 0, len(knownPublicOperationNames))
	for name := range knownPublicOperationNames {
		names = append(names, name)
	}
	slices.Sort(names)

	return names
}
