package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var errNilReadinessChecker = errors.New("readiness checker is nil")

type ReadinessChecker interface {
	CheckReadiness(context.Context) error
}

type ReadinessCheckerFunc func(context.Context) error

func (f ReadinessCheckerFunc) CheckReadiness(ctx context.Context) error {
	return f(ctx)
}

type namedReadinessChecker struct {
	name    string
	checker ReadinessChecker
}

func (c namedReadinessChecker) Name() string {
	return c.name
}

func (c namedReadinessChecker) CheckReadiness(ctx context.Context) error {
	if c.checker == nil {
		return errNilReadinessChecker
	}

	return c.checker.CheckReadiness(ctx)
}

func withReadinessCheckerName(name string, checker ReadinessChecker) ReadinessChecker {
	return namedReadinessChecker{
		name:    strings.TrimSpace(name),
		checker: checker,
	}
}

type databaseReadiness interface {
	Ping(context.Context) error
}

func defaultReadinessCheckers(database databaseReadiness) []ReadinessChecker {
	if database == nil {
		return nil
	}

	return []ReadinessChecker{
		withReadinessCheckerName("database", ReadinessCheckerFunc(func(ctx context.Context) error {
			if err := database.Ping(ctx); err != nil {
				return fmt.Errorf("database readiness check failed: %w", err)
			}

			return nil
		})),
	}
}
