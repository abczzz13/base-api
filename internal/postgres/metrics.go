package postgres

import (
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "base_api"
	metricsSubsystem = "db"
)

type poolStats interface {
	AcquireCount() int64
	AcquireDuration() time.Duration
	AcquiredConns() int32
	IdleConns() int32
	MaxConns() int32
	TotalConns() int32
}

type statsProvider interface {
	Stats() poolStats
}

type pgxPoolStatsProvider struct {
	pool *pgxpool.Pool
}

func (p pgxPoolStatsProvider) Stats() poolStats {
	return p.pool.Stat()
}

func RegisterPoolMetrics(reg prometheus.Registerer, pool *pgxpool.Pool) error {
	if reg == nil {
		return errors.New("prometheus registerer is required")
	}
	if pool == nil {
		return errors.New("database pool is required")
	}

	return registerPoolMetrics(reg, pgxPoolStatsProvider{pool: pool})
}

func registerPoolMetrics(reg prometheus.Registerer, statsProvider statsProvider) error {
	if statsProvider == nil {
		return errors.New("pool stats provider is required")
	}

	sample := func(extract func(poolStats) float64) func() float64 {
		return func() float64 {
			stats := statsProvider.Stats()
			if stats == nil {
				return 0
			}

			return extract(stats)
		}
	}

	collectors := []prometheus.Collector{
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pool_acquired_connections",
			Help:      "Current number of acquired PostgreSQL pool connections.",
		}, sample(func(stats poolStats) float64 {
			return float64(stats.AcquiredConns())
		})),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pool_idle_connections",
			Help:      "Current number of idle PostgreSQL pool connections.",
		}, sample(func(stats poolStats) float64 {
			return float64(stats.IdleConns())
		})),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pool_total_connections",
			Help:      "Current total number of PostgreSQL pool connections.",
		}, sample(func(stats poolStats) float64 {
			return float64(stats.TotalConns())
		})),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pool_max_connections",
			Help:      "Configured maximum PostgreSQL pool connections.",
		}, sample(func(stats poolStats) float64 {
			return float64(stats.MaxConns())
		})),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pool_acquire_count_total",
			Help:      "Total number of successful PostgreSQL pool acquires.",
		}, sample(func(stats poolStats) float64 {
			return float64(stats.AcquireCount())
		})),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pool_acquire_duration_seconds_total",
			Help:      "Total time spent acquiring PostgreSQL pool connections in seconds.",
		}, sample(func(stats poolStats) float64 {
			return stats.AcquireDuration().Seconds()
		})),
	}

	for _, collector := range collectors {
		if err := reg.Register(collector); err != nil {
			return fmt.Errorf("register PostgreSQL pool metrics: %w", err)
		}
	}

	return nil
}
