package publicroute

import (
	"slices"
	"strings"
)

const (
	OperationGetHealthz        = "getHealthz"
	OperationGetCurrentWeather = "getCurrentWeather"
)

var knownOperationIDs = map[string]struct{}{
	OperationGetHealthz:        {},
	OperationGetCurrentWeather: {},
}

func IsKnownOperationID(operationID string) bool {
	_, ok := knownOperationIDs[strings.TrimSpace(operationID)]
	return ok
}

func KnownOperationIDs() []string {
	ids := make([]string, 0, len(knownOperationIDs))
	for operationID := range knownOperationIDs {
		ids = append(ids, operationID)
	}
	slices.Sort(ids)

	return ids
}
