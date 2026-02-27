package telemetry

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestBuildResource(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want map[string]string
	}{
		{
			name: "uses default service name when unset",
			cfg:  Config{},
			want: map[string]string{
				"service.name": defaultServiceName,
			},
		},
		{
			name: "includes configured service metadata",
			cfg: Config{
				ServiceName:    "  custom-service  ",
				ServiceVersion: "1.2.3",
				Environment:    "production",
			},
			want: map[string]string{
				"service.name":                "custom-service",
				"service.version":             "1.2.3",
				"deployment.environment.name": "production",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := buildResource(tt.cfg)
			if err != nil {
				t.Fatalf("buildResource returned error: %v", err)
			}

			got := map[string]string{}
			for _, attr := range res.Attributes() {
				if attr.Value.Type() != attribute.STRING {
					continue
				}

				got[string(attr.Key)] = attr.Value.AsString()
			}

			for key, wantValue := range tt.want {
				if diff := cmp.Diff(wantValue, got[key]); diff != "" {
					t.Fatalf("attribute %q mismatch (-want +got):\n%s", key, diff)
				}
			}
		})
	}
}

func TestInitTracing(t *testing.T) {
	resetOTLPEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")

	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	shutdown, err := InitTracing(context.Background(), Config{ServiceName: "test-service"})
	if err != nil {
		t.Fatalf("InitTracing returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracing returned nil shutdown function")
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
}

func TestInitTracingCancelledContextReturnsError(t *testing.T) {
	resetOTLPEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	shutdown, err := InitTracing(ctx, Config{ServiceName: "test-service"})
	if err == nil {
		t.Fatal("InitTracing returned nil error for canceled context")
	}
	if shutdown != nil {
		t.Fatal("InitTracing returned shutdown function on error")
	}
}

func TestBuildSampler(t *testing.T) {
	traceID := trace.TraceID{1}
	unsampledParentContext := trace.ContextWithSpanContext(
		context.Background(),
		trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    trace.TraceID{2},
			SpanID:     trace.SpanID{2},
			TraceFlags: 0,
			Remote:     true,
		}),
	)

	tests := []struct {
		name          string
		samplerName   TraceSampler
		samplerArg    *float64
		parentContext context.Context
		want          sdktrace.SamplingDecision
	}{
		{
			name:          "defaults to parentbased always on for root spans",
			samplerName:   "",
			parentContext: context.Background(),
			want:          sdktrace.RecordAndSample,
		},
		{
			name:          "default sampler honors unsampled parent",
			samplerName:   "",
			parentContext: unsampledParentContext,
			want:          sdktrace.Drop,
		},
		{
			name:          "always on ignores unsampled parent",
			samplerName:   "always_on",
			parentContext: unsampledParentContext,
			want:          sdktrace.RecordAndSample,
		},
		{
			name:          "always off drops spans",
			samplerName:   "always_off",
			parentContext: context.Background(),
			want:          sdktrace.Drop,
		},
		{
			name:          "trace id ratio uses configured argument",
			samplerName:   "traceidratio",
			samplerArg:    float64Ptr(0),
			parentContext: context.Background(),
			want:          sdktrace.Drop,
		},
		{
			name:          "invalid sampler falls back to parentbased always on",
			samplerName:   TraceSampler("not-real"),
			parentContext: unsampledParentContext,
			want:          sdktrace.Drop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sampler := buildSampler(tt.samplerName, tt.samplerArg)
			result := sampler.ShouldSample(sdktrace.SamplingParameters{
				ParentContext: tt.parentContext,
				TraceID:       traceID,
				Name:          "test-span",
				Kind:          trace.SpanKindServer,
			})

			if diff := cmp.Diff(tt.want, result.Decision); diff != "" {
				t.Fatalf("sampling decision mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseTraceSampler(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  TraceSampler
		ok    bool
	}{
		{name: "valid sampler is normalized", input: " ParentBased_TraceIDRatio ", want: TraceSamplerParentBasedTraceIDRatio, ok: true},
		{name: "invalid sampler returns false", input: "not-real", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseTraceSampler(tt.input)
			if diff := cmp.Diff(tt.ok, ok); diff != "" {
				t.Fatalf("validity mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("parsed sampler mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTraceSamplerUsesArgument(t *testing.T) {
	tests := []struct {
		name    string
		sampler TraceSampler
		want    bool
	}{
		{name: "traceidratio uses argument", sampler: TraceSamplerTraceIDRatio, want: true},
		{name: "parentbased traceidratio uses argument", sampler: TraceSamplerParentBasedTraceIDRatio, want: true},
		{name: "always on does not use argument", sampler: TraceSamplerAlwaysOn, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.sampler.UsesArgument()); diff != "" {
				t.Fatalf("UsesArgument mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func resetOTLPEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_HEADERS",
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL",
		"OTEL_EXPORTER_OTLP_TIMEOUT",
		"OTEL_EXPORTER_OTLP_TRACES_TIMEOUT",
		"OTEL_EXPORTER_OTLP_INSECURE",
		"OTEL_EXPORTER_OTLP_TRACES_INSECURE",
		"OTEL_EXPORTER_OTLP_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_CLIENT_KEY",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY",
		"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE",
	} {
		t.Setenv(key, "")
	}
}
