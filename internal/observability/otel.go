// Package observability wires OpenTelemetry tracing and Prometheus metrics
// for every platform component (§31). Integrations under
// internal/integrations and internal/infrastructure instrument their calls
// using the tracer/meter constructed here rather than each rolling their own.
package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Shutdown flushes and stops the tracer provider; call it on process exit.
type Shutdown func(context.Context) error

// InitTracing configures the global OpenTelemetry tracer provider to export
// spans to an OTLP collector at endpoint, tagged with serviceName. When
// enabled is false, a no-op provider is installed so instrumentation code
// pays no cost and needs no branching.
func InitTracing(ctx context.Context, serviceName, endpoint string, enabled bool) (Shutdown, error) {
	if !enabled {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, fmt.Errorf("building OTel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// Tracer returns a named tracer from the globally configured provider.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// InitMetrics configures the global OpenTelemetry meter provider to export
// metrics to the same OTLP collector traces go to, tagged with serviceName
// (§31: correlating traces and metrics, both exported to the same backend).
// Once set, otelhttp's server/client instrumentation (already wrapping the
// API's HTTP handler and every real integration client's transport, per
// their own per-call spans) automatically emits request duration/count
// metrics through it with no further code at each call site. When enabled
// is false, a no-op provider is installed.
func InitMetrics(ctx context.Context, serviceName, endpoint string, enabled bool) (Shutdown, error) {
	if !enabled {
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpoint(endpoint), otlpmetricgrpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("creating OTLP metric exporter: %w", err)
	}

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, fmt.Errorf("building OTel resource: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return mp.Shutdown, nil
}

// Meter returns a named meter from the globally configured provider.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}
