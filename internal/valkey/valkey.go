package valkey

import (
	"context"
	"errors"
	"fmt"
	"strings"

	valkey "github.com/valkey-io/valkey-go"
)

// Client is a type alias for the Valkey driver client, re-exported for convenience.
type Client = valkey.Client

type Mode string

const (
	ModeStandalone Mode = "standalone"
	ModeCluster    Mode = "cluster"
)

type Config struct {
	Mode  Mode
	Addrs []string
}

func (c Config) NormalizedMode() Mode {
	mode := Mode(strings.ToLower(strings.TrimSpace(string(c.Mode))))
	if mode == "" {
		return ModeStandalone
	}

	return mode
}

func (c Config) ValidateMode() error {
	switch c.NormalizedMode() {
	case ModeStandalone, ModeCluster:
		return nil
	default:
		return fmt.Errorf("unsupported mode %q", c.Mode)
	}
}

// PingableClient wraps a Client with a Ping method for readiness checks.
type PingableClient struct {
	Client
}

// NewPingableClient wraps a Client so it satisfies readiness-check interfaces.
func NewPingableClient(c Client) *PingableClient {
	return &PingableClient{Client: c}
}

// Ping sends a PING command to Valkey and returns any error.
func (p *PingableClient) Ping(ctx context.Context) error {
	return p.Do(ctx, p.B().Ping().Build()).Error()
}

func NewClient(cfg Config) (Client, error) {
	if err := cfg.ValidateMode(); err != nil {
		return nil, fmt.Errorf("validate mode: %w", err)
	}
	if len(cfg.Addrs) == 0 {
		return nil, errors.New("at least one Valkey address is required")
	}

	option := valkey.ClientOption{InitAddress: cfg.Addrs}
	if cfg.NormalizedMode() == ModeCluster {
		option.ShuffleInit = true
	}

	client, err := valkey.NewClient(option)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	return client, nil
}
