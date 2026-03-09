package valkey

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestConfigNormalizedMode(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		want Mode
	}{
		{
			name: "defaults empty mode to standalone",
			mode: "",
			want: ModeStandalone,
		},
		{
			name: "trims and normalizes cluster mode",
			mode: "  CLUSTER ",
			want: ModeCluster,
		},
		{
			name: "keeps standalone mode",
			mode: ModeStandalone,
			want: ModeStandalone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Mode: tt.mode}
			if diff := cmp.Diff(tt.want, cfg.NormalizedMode()); diff != "" {
				t.Fatalf("NormalizedMode mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigValidateMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    Mode
		wantErr string
	}{
		{
			name: "accepts standalone",
			mode: ModeStandalone,
		},
		{
			name: "accepts cluster regardless of case",
			mode: "ClUsTeR",
		},
		{
			name:    "rejects unsupported mode",
			mode:    "sentinel",
			wantErr: `unsupported mode "sentinel"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Config{Mode: tt.mode}.ValidateMode()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateMode returned error: %v", err)
				}
				return
			}

			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("ValidateMode error mismatch: want %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestNewClientRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		wantErrPart string
	}{
		{
			name:        "rejects invalid mode",
			cfg:         Config{Mode: "invalid", Addrs: []string{"127.0.0.1:6379"}},
			wantErrPart: `validate mode: unsupported mode "invalid"`,
		},
		{
			name:        "rejects missing addresses",
			cfg:         Config{Mode: ModeStandalone},
			wantErrPart: "at least one Valkey address is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.cfg)
			if client != nil {
				t.Fatalf("expected nil client, got %T", client)
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErrPart) {
				t.Fatalf("NewClient error mismatch: want substring %q, got %v", tt.wantErrPart, err)
			}
		})
	}
}
