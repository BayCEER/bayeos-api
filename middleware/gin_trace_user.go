package middleware

import (
	"strconv"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/semconv/v1.39.0"
	"go.opentelemetry.io/otel/trace"
)

// TraceUser attaches the authenticated user to the current request span
// using the OpenTelemetry user semantic conventions (user.id, user.name).
// Must run after AuthUser has stored the user in the context.
func TraceUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, exists := c.Get("User")
		if !exists {
			c.Next()
			return
		}
		user := u.(model.User)
		trace.SpanFromContext(c.Request.Context()).SetAttributes(
			semconv.UserID(strconv.FormatInt(int64(user.ID), 10)),
			semconv.UserName(user.SignInName),
		)
		c.Next()
	}
}