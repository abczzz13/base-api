package server

import (
	"context"

	"github.com/abczzz13/base-api/internal/config"
)

type ReadinessChecker interface {
	CheckReadiness(context.Context) error
}

type ReadinessCheckerFunc func(context.Context) error

func (f ReadinessCheckerFunc) CheckReadiness(ctx context.Context) error {
	return f(ctx)
}

func defaultReadinessCheckers(cfg config.Config) []ReadinessChecker {
	_ = cfg

	return nil
}
