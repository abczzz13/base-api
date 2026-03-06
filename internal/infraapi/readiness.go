package infraapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNilReadinessChecker indicates a nil readiness checker.
var ErrNilReadinessChecker = errors.New("readiness checker is nil")

// ReadinessChecker checks whether a dependency is ready.
type ReadinessChecker interface {
	CheckReadiness(context.Context) error
}

// ReadinessCheckerFunc adapts a function into a ReadinessChecker.
type ReadinessCheckerFunc func(context.Context) error

// CheckReadiness implements ReadinessChecker.
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
		return ErrNilReadinessChecker
	}

	return c.checker.CheckReadiness(ctx)
}

// WithReadinessCheckerName labels a readiness checker for logs.
func WithReadinessCheckerName(name string, checker ReadinessChecker) ReadinessChecker {
	return namedReadinessChecker{
		name:    strings.TrimSpace(name),
		checker: checker,
	}
}

// DatabaseReadiness is the readiness contract for database dependencies.
type DatabaseReadiness interface {
	Ping(context.Context) error
}

// DefaultReadinessCheckers builds default readiness checks from configured dependencies.
func DefaultReadinessCheckers(database DatabaseReadiness) []ReadinessChecker {
	if database == nil {
		return nil
	}

	return []ReadinessChecker{
		WithReadinessCheckerName("database", ReadinessCheckerFunc(func(ctx context.Context) error {
			if err := database.Ping(ctx); err != nil {
				return fmt.Errorf("database readiness check failed: %w", err)
			}

			return nil
		})),
	}
}

// ReadinessCheckerLogName returns a stable log identifier for a readiness checker.
func ReadinessCheckerLogName(checker ReadinessChecker, index int) string {
	if checker == nil {
		return fmt.Sprintf("checker_%d", index)
	}

	namedChecker, ok := checker.(interface{ Name() string })
	if !ok {
		return fmt.Sprintf("checker_%d", index)
	}

	name := strings.TrimSpace(namedChecker.Name())
	if name == "" {
		return fmt.Sprintf("checker_%d", index)
	}

	return name
}
