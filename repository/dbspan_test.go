package repository

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

func TestWithQuerySpanAttributes(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(sdktrace.NewTracerProvider())

	ctx, span := withQuerySpan(context.Background(), "SELECT id FROM node WHERE id=$1")
	span.End()
	// force the recorder to capture
	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatalf("force flush: %v", err)
	}

	got := rec.Ended()
	if len(got) != 1 {
		t.Fatalf("expected 1 span, got %d", len(got))
	}
	rs := got[0]

	if got2 := rs.Name(); got2 != "db.query" {
		t.Errorf("span name = %q, want %q", got2, "db.query")
	}

	attrs := make(map[string]string)
	for _, kv := range rs.Attributes() {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}

	if attrs[string(semconv.DBSystemNameKey)] != "postgresql" {
		t.Errorf("db.system.name = %q, want %q", attrs[string(semconv.DBSystemNameKey)], "postgresql")
	}
	if attrs[string(semconv.DBOperationNameKey)] != "SELECT" {
		t.Errorf("db.operation.name = %q, want %q", attrs[string(semconv.DBOperationNameKey)], "SELECT")
	}
	if attrs[string(semconv.DBQueryTextKey)] != "SELECT id FROM node WHERE id=$1" {
		t.Errorf("db.query.text = %q, want %q", attrs[string(semconv.DBQueryTextKey)], "SELECT id FROM node WHERE id=$1")
	}
}

func TestEndQuerySpanRecordsError(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(sdktrace.NewTracerProvider())

	_, span := withQuerySpan(context.Background(), "SELECT id FROM node WHERE id=$1")
	sentinel := errors.New("db down")
	endQuerySpan(span, sentinel)

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("force flush: %v", err)
	}

	got := rec.Ended()
	if len(got) != 1 {
		t.Fatalf("expected 1 span, got %d", len(got))
	}
	rs := got[0]
	if rs.Status().Code.String() != "Error" {
		t.Errorf("span status = %v, want Error", rs.Status().Code)
	}
}

func TestDBOperation(t *testing.T) {
	cases := []struct {
		stmt string
		want string
	}{
		{"SELECT * FROM node", "SELECT"},
		{"insert into node values(1)", "INSERT"},
		{"  UPDATE node set x=1", "UPDATE"},
		{"DELETE FROM node", "DELETE"},
		{"WITH recursive wf AS (...)", "WITH"},
		{"CREATE TABLE x(...)", "CREATE"},
		{"ALTER TABLE x", "ALTER"},
		{"DROP TABLE x", "DROP"},
		{"", "query"},
		{"not an sql statement", "query"},
	}
	for _, c := range cases {
		if got := dbOperation(c.stmt); got != c.want {
			t.Errorf("dbOperation(%q) = %q, want %q", c.stmt, got, c.want)
		}
	}
}
