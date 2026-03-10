package infraapi

import (
	"context"
	"fmt"
	"strings"
)

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

// ValkeyReadiness is the readiness contract for Valkey dependencies.
type ValkeyReadiness interface {
	Ping(context.Context) error
}

// DefaultReadinessCheckers builds default readiness checks from configured dependencies.
func DefaultReadinessCheckers(database DatabaseReadiness, valkey ValkeyReadiness) []ReadinessChecker {
	var checkers []ReadinessChecker
	if database != nil {
		checkers = append(checkers, WithReadinessCheckerName("database", ReadinessCheckerFunc(database.Ping)))
	}
	if valkey != nil {
		checkers = append(checkers, WithReadinessCheckerName("valkey", ReadinessCheckerFunc(valkey.Ping)))
	}
	return checkers
}

// ReadinessCheckerLogName returns a stable log identifier for a readiness checker.
func ReadinessCheckerLogName(checker ReadinessChecker, index int) string {
	if named, ok := checker.(interface{ Name() string }); ok {
		if name := strings.TrimSpace(named.Name()); name != "" {
			return name
		}
	}

	return fmt.Sprintf("checker_%d", index)
}
