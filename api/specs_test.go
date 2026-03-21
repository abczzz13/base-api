package api

import (
	"bytes"
	"testing"
)

func TestOpenAPISpecYAMLGettersReturnCopies(t *testing.T) {
	tests := []struct {
		name string
		get  func() []byte
	}{
		{
			name: "public spec",
			get:  PublicOpenAPISpecYAML,
		},
		{
			name: "weather spec",
			get:  WeatherOpenAPISpecYAML,
		},
		{
			name: "infra spec",
			get:  InfraOpenAPISpecYAML,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.get()
			if len(original) == 0 {
				t.Fatalf("embedded spec should not be empty")
			}

			mutated := tt.get()
			mutated[0] ^= 0xff

			afterMutation := tt.get()
			if !bytes.Equal(original, afterMutation) {
				t.Fatalf("mutating returned bytes changed embedded spec")
			}
		})
	}
}
