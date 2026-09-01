package repository

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "bayeos-api/repository"

// withQuerySpan starts a database query span for the given statement, returning
// the span so the caller can End it (deferred) and record errors on it.
// The returned span is a child of any span already present in ctx (e.g. the
// HTTP request span created by otelgin).
func withQuerySpan(ctx context.Context, statement string) (context.Context, trace.Span) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "db.query")
	span.SetAttributes(
		semconv.DBSystemNamePostgreSQL,
		semconv.DBOperationName(dbOperation(statement)),
		attribute.String("db.query.text", statement),
	)
	return ctx, span
}

// endQuerySpan records the query result on the span and ends it.
func endQuerySpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// dbOperation extracts the leading SQL verb (SELECT/INSERT/UPDATE/DELETE) from a
// statement, used for the db.operation.name attribute. Defaults to "query".
func dbOperation(statement string) string {
	s := strings.TrimSpace(statement)
	i := strings.IndexAny(s, " \t\n")
	if i < 0 {
		i = len(s)
	}
	verb := strings.ToUpper(s[:i])
	switch verb {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "WITH", "CREATE", "ALTER", "DROP":
		return verb
	default:
		return "query"
	}
}
