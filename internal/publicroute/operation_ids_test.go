package publicroute

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var operationIDPattern = regexp.MustCompile(`(?m)^\s*operationId:\s*([[:alnum:]_"'-]+)\s*$`)

func TestKnownOperationIDsStayAlignedWithOpenAPISpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "api", "openapi.yaml")
	//nolint:gosec // Test reads a fixed repository-local spec path.
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read OpenAPI spec: %v", err)
	}

	matches := operationIDPattern.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("expected at least one operationId in OpenAPI spec")
	}

	fromSpec := make([]string, 0, len(matches))
	for _, match := range matches {
		fromSpec = append(fromSpec, trimYAMLScalar(match[1]))
	}
	slices.Sort(fromSpec)

	if diff := cmp.Diff(fromSpec, KnownOperationIDs()); diff != "" {
		t.Fatalf("known operation IDs drifted from api/openapi.yaml (-want +got):\n%s", diff)
	}
}

func trimYAMLScalar(value string) string {
	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			return value[1 : len(value)-1]
		}
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return value[1 : len(value)-1]
		}
	}

	return value
}
