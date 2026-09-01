package repository

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// fakeDBTX always fails GetContext with the given error, so a failing DB query
// can be simulated without a live database.
type fakeDBTX struct {
	err error
}

func (f *fakeDBTX) Exec(string, ...interface{}) (sql.Result, error)            { return nil, f.err }
func (f *fakeDBTX) NamedExec(string, interface{}) (sql.Result, error)          { return nil, f.err }
func (f *fakeDBTX) Get(interface{}, string, ...interface{}) error              { return f.err }
func (f *fakeDBTX) Select(interface{}, string, ...interface{}) error           { return f.err }
func (f *fakeDBTX) ExecContext(context.Context, string, ...interface{}) (sql.Result, error) {
	return nil, f.err
}
func (f *fakeDBTX) GetContext(context.Context, interface{}, string, ...interface{}) error {
	return f.err
}
func (f *fakeDBTX) SelectContext(context.Context, interface{}, string, ...interface{}) error {
	return f.err
}

// TestDBSpanParentsToHandlerSpanOnError reproduces the orphaned-span symptom: a
// failing SQL query emitted through a request handled by otelgin must appear as
// a child of the HTTP handler span, not as a root span.
func TestDBSpanParentsToHandlerSpanOnError(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(sdktrace.NewTracerProvider())

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(otelgin.Middleware("test-service"))

	repo := SqlRepo{trans: &fakeDBTX{err: errors.New("db down")}}

	r.GET("/boom", func(c *gin.Context) {
		// Deliberately propagate the request context (the one otelgin populated)
		// so the failing query's span must be parented to the HTTP span.
		if _, err := repo.FindUserByID(c.Request.Context(), 1); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("force flush: %v", err)
	}

	spans := rec.Ended()
	var httpSpan, dbSpan sdktrace.ReadOnlySpan
	for _, s := range spans {
		switch {
		case s.Name() == "db.query":
			dbSpan = s
		case strings.HasPrefix(s.Name(), "GET"):
			httpSpan = s
		}
	}
	if httpSpan == nil {
		t.Fatalf("expected an otelgin HTTP span, got spans: %v", spanNames(spans))
	}
	if dbSpan == nil {
		t.Fatalf("expected a db.query span, got spans: %v", spanNames(spans))
	}

	httpSC := httpSpan.SpanContext()
	dbSC := dbSpan.SpanContext()
	if httpSC.TraceID() != dbSC.TraceID() {
		t.Errorf("db span trace %s != handler span trace %s (orphaned trace)", dbSC.TraceID(), httpSC.TraceID())
	}
	if dbSC.SpanID() == httpSC.SpanID() {
		t.Fatalf("db span and handler span share the same SpanID; expected db span to be a child")
	}
	if dbSpan.Parent().SpanID() != httpSC.SpanID() {
		t.Errorf("db span parent %s != handler span %s (db span is not parented to the handler span)", dbSpan.Parent().SpanID(), httpSC.SpanID())
	}

	// The db span must not be a root: its parent must be valid.
	if !dbSpan.Parent().IsValid() {
		t.Errorf("db span parent is invalid (root/orphan)")
	}
}

func spanNames(spans []sdktrace.ReadOnlySpan) []string {
	names := make([]string, 0, len(spans))
	for _, s := range spans {
		names = append(names, s.Name())
	}
	return names
}
