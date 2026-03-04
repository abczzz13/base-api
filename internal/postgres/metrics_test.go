package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRegisterPoolMetricsValidation(t *testing.T) {
	tests := []struct {
		name         string
		call         func() error
		wantContains string
	}{
		{
			name: "fails with nil registerer",
			call: func() error {
				return RegisterPoolMetrics(nil, nil)
			},
			wantContains: "prometheus registerer is required",
		},
		{
			name: "fails with nil pool",
			call: func() error {
				return RegisterPoolMetrics(prometheus.NewRegistry(), nil)
			},
			wantContains: "database pool is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("RegisterPoolMetrics returned nil error")
			}
			if got := err.Error(); got != tt.wantContains {
				t.Fatalf("error mismatch: want %q, got %q", tt.wantContains, got)
			}
		})
	}
}

func TestRegisterPoolMetrics(t *testing.T) {
	tests := []struct {
		name         string
		provider     statsProvider
		wantContains string
		assert       func(*testing.T, []*dto.MetricFamily)
	}{
		{
			name:         "fails with nil provider",
			provider:     nil,
			wantContains: "pool stats provider is required",
		},
		{
			name: "registers and exposes pool metrics",
			provider: fakeStatsProvider{stats: fakePoolStats{
				acquireCount:    12,
				acquireDuration: 1500 * time.Millisecond,
				acquiredConns:   3,
				idleConns:       5,
				totalConns:      9,
				maxConns:        20,
			}},
			assert: func(t *testing.T, metricFamilies []*dto.MetricFamily) {
				t.Helper()

				if got := metricValue(t, metricFamilies, "base_api_db_pool_acquired_connections"); got != 3 {
					t.Fatalf("acquired connections mismatch: want 3, got %v", got)
				}
				if got := metricValue(t, metricFamilies, "base_api_db_pool_idle_connections"); got != 5 {
					t.Fatalf("idle connections mismatch: want 5, got %v", got)
				}
				if got := metricValue(t, metricFamilies, "base_api_db_pool_total_connections"); got != 9 {
					t.Fatalf("total connections mismatch: want 9, got %v", got)
				}
				if got := metricValue(t, metricFamilies, "base_api_db_pool_max_connections"); got != 20 {
					t.Fatalf("max connections mismatch: want 20, got %v", got)
				}
				if got := metricValue(t, metricFamilies, "base_api_db_pool_acquire_count_total"); got != 12 {
					t.Fatalf("acquire count mismatch: want 12, got %v", got)
				}
				if got := metricValue(t, metricFamilies, "base_api_db_pool_acquire_duration_seconds_total"); got != 1.5 {
					t.Fatalf("acquire duration mismatch: want 1.5, got %v", got)
				}
			},
		},
		{
			name: "duplicate registration fails",
			provider: fakeStatsProvider{stats: fakePoolStats{
				acquireCount:    1,
				acquireDuration: time.Second,
				acquiredConns:   1,
				idleConns:       1,
				totalConns:      2,
				maxConns:        10,
			}},
			wantContains: "register PostgreSQL pool metrics",
			assert:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()

			err := registerPoolMetrics(reg, tt.provider)
			if tt.name == "duplicate registration fails" {
				if err != nil {
					t.Fatalf("initial registerPoolMetrics returned error: %v", err)
				}
				err = registerPoolMetrics(reg, tt.provider)
			}

			if tt.wantContains != "" {
				if err == nil {
					t.Fatalf("registerPoolMetrics returned nil error")
				}
				if !strings.Contains(err.Error(), tt.wantContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("registerPoolMetrics returned error: %v", err)
			}

			metricFamilies, err := reg.Gather()
			if err != nil {
				t.Fatalf("gather metrics: %v", err)
			}

			if tt.assert != nil {
				tt.assert(t, metricFamilies)
			}
		})
	}
}

func metricValue(t *testing.T, metricFamilies []*dto.MetricFamily, name string) float64 {
	t.Helper()

	for _, family := range metricFamilies {
		if family.GetName() != name {
			continue
		}

		metrics := family.GetMetric()
		if len(metrics) != 1 {
			t.Fatalf("metric %q has %d samples, want 1", name, len(metrics))
		}

		metric := metrics[0]
		if counter := metric.GetCounter(); counter != nil {
			return counter.GetValue()
		}
		if gauge := metric.GetGauge(); gauge != nil {
			return gauge.GetValue()
		}

		t.Fatalf("metric %q has unsupported type", name)
	}

	t.Fatalf("metric %q not found", name)
	return 0
}

type fakeStatsProvider struct {
	stats poolStats
}

func (f fakeStatsProvider) Stats() poolStats {
	return f.stats
}

type fakePoolStats struct {
	acquireCount    int64
	acquireDuration time.Duration
	acquiredConns   int32
	idleConns       int32
	maxConns        int32
	totalConns      int32
}

func (f fakePoolStats) AcquireCount() int64 { return f.acquireCount }

func (f fakePoolStats) AcquireDuration() time.Duration { return f.acquireDuration }

func (f fakePoolStats) AcquiredConns() int32 { return f.acquiredConns }

func (f fakePoolStats) IdleConns() int32 { return f.idleConns }

func (f fakePoolStats) MaxConns() int32 { return f.maxConns }

func (f fakePoolStats) TotalConns() int32 { return f.totalConns }
