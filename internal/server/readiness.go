package server

import "context"

type ReadinessChecker interface {
	CheckReadiness(context.Context) error
}

type ReadinessCheckerFunc func(context.Context) error

func (f ReadinessCheckerFunc) CheckReadiness(ctx context.Context) error {
	return f(ctx)
}

func defaultReadinessCheckers(cfg Config) []ReadinessChecker {
	_ = cfg

	return nil
}
