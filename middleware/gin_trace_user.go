package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

// TraceUser attaches the authenticated user to the current request span
// using the OpenTelemetry user semantic conventions (user.id, user.name).
// Must run after AuthUser has stored the user in the context.
func TraceUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, exists := AuthUserFromContext(c)
		if !exists {
			c.Next()
			return
		}
		trace.SpanFromContext(c.Request.Context()).SetAttributes(
			semconv.UserID(strconv.FormatInt(int64(u.ID), 10)),
			semconv.UserName(u.SignInName),
		)
		c.Next()
	}
}
