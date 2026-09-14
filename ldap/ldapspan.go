package ldap

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "bayeos-api/ldap"

// withLdapSpan starts an LDAP operation span, returning the span so the caller
// can End it (deferred) and record errors on it. The returned span is a child
// of any span already present in ctx (e.g. the users.sign_in service span).
func withLdapSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, name)
	span.SetAttributes(semconv.DBSystemNameKey.String("ldap"))
	return ctx, span
}

// endLdapSpan records the operation result on the span and ends it.
func endLdapSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}