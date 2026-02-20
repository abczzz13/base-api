package server

import "time"

type Config struct {
	Address       string
	InfraAddress  string
	Environment   string
	ReadyzTimeout time.Duration
}

func loadConfig(getenv func(string) string) Config {
	cfg := Config{
		Address:      getenv("API_ADDR"),
		InfraAddress: getenv("API_INFRA_ADDR"),
		Environment:  getenv("API_ENVIRONMENT"),
	}

	if cfg.Address == "" {
		cfg.Address = ":8080"
	}

	if cfg.InfraAddress == "" {
		cfg.InfraAddress = "127.0.0.1:9090"
	}

	if cfg.Environment == "" {
		cfg.Environment = getenv("APP_ENV")
	}

	if cfg.Environment == "" {
		cfg.Environment = getenv("ENVIRONMENT")
	}

	if cfg.Environment == "" {
		cfg.Environment = "development"
	}

	cfg.ReadyzTimeout = 2 * time.Second
	if timeout := getenv("API_READYZ_TIMEOUT"); timeout != "" {
		if parsed, err := time.ParseDuration(timeout); err == nil && parsed > 0 {
			cfg.ReadyzTimeout = parsed
		}
	}

	return cfg
}
