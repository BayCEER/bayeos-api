package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTraceUser(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(CtxAuthUser, model.User{ID: 42, SignInName: "barti"})

	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

	TraceUser()(c)
	span.End()

	spans := sr.Ended()
	require.Len(t, spans, 1)
	attrs := make(map[attribute.Key]string)
	for _, kv := range spans[0].Attributes() {
		attrs[kv.Key] = kv.Value.String()
	}
	require.Equal(t, "42", attrs["user.id"])
	require.Equal(t, "barti", attrs["user.name"])
}

func TestTraceUserNoUser(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

	TraceUser()(c)
	span.End()

	spans := sr.Ended()
	require.Len(t, spans, 1)
	require.Len(t, spans[0].Attributes(), 0)
}
