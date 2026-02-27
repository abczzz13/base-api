package telemetry

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)

const defaultServiceName = "base-api"

type Config struct {
	ServiceName      string
	ServiceVersion   string
	Environment      string
	TracesSampler    TraceSampler
	TracesSamplerArg *float64
}

func InitTracing(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	res, err := buildResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("build OpenTelemetry resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(buildSampler(cfg.TracesSampler, cfg.TracesSamplerArg)),
	)

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return provider.Shutdown, nil
}

func buildSampler(name TraceSampler, arg *float64) sdktrace.Sampler {
	ratio := 1.0
	if arg != nil && *arg >= 0 && *arg <= 1 {
		ratio = *arg
	}

	switch name {
	case TraceSamplerAlwaysOn:
		return sdktrace.AlwaysSample()
	case TraceSamplerAlwaysOff:
		return sdktrace.NeverSample()
	case TraceSamplerTraceIDRatio:
		return sdktrace.TraceIDRatioBased(ratio)
	case "", TraceSamplerParentBasedAlwaysOn:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case TraceSamplerParentBasedAlwaysOff:
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case TraceSamplerParentBasedTraceIDRatio:
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

func buildResource(cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName(cfg.ServiceName)),
	}

	if serviceVersion := strings.TrimSpace(cfg.ServiceVersion); serviceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(serviceVersion))
	}
	if environment := strings.TrimSpace(cfg.Environment); environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentName(environment))
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("merge resources: %w", err)
	}

	return res, nil
}

func serviceName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultServiceName
	}

	return trimmed
}
