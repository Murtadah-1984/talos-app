package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/talos-platform/talos-platform/internal/observability"
)

// otelQueryTracer implements pgx.QueryTracer, emitting one span per
// Query/QueryRow/Exec call (§31 full OTel coverage). It's hand-rolled rather
// than pulling in a third-party pgx-OTel package: the interface is three
// methods and the platform already has a tracer helper in
// internal/observability.
type otelQueryTracer struct {
	tracer trace.Tracer
}

// newOTelQueryTracer returns a pgx.QueryTracer that reports spans under the
// "postgres" tracer name.
func newOTelQueryTracer() *otelQueryTracer {
	return &otelQueryTracer{tracer: observability.Tracer("postgres")}
}

type traceSpanKey struct{}

func (t *otelQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx, span := t.tracer.Start(ctx, "postgres.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.statement", data.SQL),
	))
	return context.WithValue(ctx, traceSpanKey{}, span)
}

func (t *otelQueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(traceSpanKey{}).(trace.Span)
	if !ok {
		return
	}
	if data.Err != nil {
		span.SetStatus(codes.Error, data.Err.Error())
		span.RecordError(data.Err)
	}
	span.End()
}
