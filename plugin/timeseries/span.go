package timeseries

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "bayeos-api/timeseries"

// withServiceSpan starts a child span for a timeseries service operation. The
// returned span is a child of any span already present in ctx, e.g. the HTTP
// request span created by otelgin.
func withServiceSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name)
}

// endServiceSpan records the operation result on the span and ends it. It
// should be invoked via defer from instrumented service methods so that errors
// are recorded on the span and surface to Jaeger with status=Error.
func endServiceSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// setNodeID records the node id on the span when known at call time.
func setNodeID(span trace.Span, id int32) {
	span.SetAttributes(attribute.Int64("bayeos.node_id", int64(id)))
}

// setUserID records the acting user id on the span.
func setUserID(span trace.Span, id int32) {
	span.SetAttributes(attribute.Int64("bayeos.user_id", int64(id)))
}
