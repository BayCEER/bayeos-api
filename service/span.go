package service

import (
	"context"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "bayeos-api/service"

// withServiceSpan starts a child span for a service operation. The returned
// span is a child of any span already present in ctx, e.g. the HTTP request
// span created by otelgin.
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

// setAccess records the acting subject on the span. A user subject is recorded
// as bayeos.user_id; a node-token subject has no user id and is marked with
// bayeos.node_token instead.
func setAccess(span trace.Span, access model.NodeAccess) {
	if u, ok := access.(model.UserScoped); ok {
		span.SetAttributes(attribute.Int64("bayeos.user_id", int64(u.UserID())))
		return
	}
	span.SetAttributes(attribute.Bool("bayeos.node_token", true))
}
