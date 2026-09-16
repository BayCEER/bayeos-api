package middleware

// Package httperror provides a Gin middleware that translates errors raised
// by handlers into the ErrorResponse JSON body defined in
// assets/openapi/schema/ErrorResponse.yaml and referenced by every
// operation's "default" response via components/GenericError.yaml.

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/BayCEER/bayeos/bayeos-api/repository"
	log "github.com/BayCEER/bayeos/bayeos-api/logger"
)

// APIError is a machine-readable error matching the ErrorResponse schema.
type APIError struct {
	Status  int            `json:"-"`
	Code    string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return e.Message
}

// New creates an APIError with the given HTTP status, machine-readable code and message.
func New(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

// WithDetails attaches additional context to the error.
func (e *APIError) WithDetails(details map[string]any) *APIError {
	e.Details = details
	return e
}

func AbortWithInternalError(c *gin.Context, message string) {
	Abort(c, New(http.StatusInternalServerError, "InternalError", message))
}

func AbortWithBadRequestError(c *gin.Context, message string) {
	Abort(c, New(http.StatusBadRequest, "BadRequest", message))
}

func AbortWithNotFoundError(c *gin.Context, resource, field, value string) {
	Abort(c, &APIError{
		Status:  http.StatusNotFound,
		Code:    "NotFound",
		Message: "Resource not found",
		Details: map[string]any{
			field:      value,
			"resource": resource,
		},
	})
}

func AbortWithUnauthorizedError(c *gin.Context, message string) {
	Abort(c, New(http.StatusUnauthorized, "Unauthorized", message))
}

// AbortWithNotFoundIfNoRows aborts with a 404 and returns true if err wraps sql.ErrNoRows (e.g. a delete/update matched no rows).
func AbortWithNotFoundIfNoRows(c *gin.Context, err error, resource, field, value string) bool {
	if errors.Is(err, sql.ErrNoRows) {
		AbortWithNotFoundError(c, resource, field, value)
		return true
	}
	return false
}

// AbortWithBadRequestIfReadOnlyField aborts with a 400 and returns true if err
// wraps the repository read-only column error.
func AbortWithBadRequestIfReadOnlyField(c *gin.Context, err error) bool {
	if errors.Is(err, repository.ErrReadOnlyField) {
		AbortWithBadRequestError(c, "Invalid request body: read-only field provided")
		return true
	}
	return false
}

func AbortWithForbiddenError(c *gin.Context, message string) {
	Abort(c, New(http.StatusForbidden, "Forbidden", message))
}

// Abort registers err on the context and stops the handler chain; the
// response is written by Middleware once the chain unwinds.
func Abort(c *gin.Context, err *APIError) {
	c.Error(err)
	c.Abort()
}

// response mirrors ErrorResponse.yaml.
type response struct {
	Error     string         `json:"error"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Timestamp string         `json:"timestamp"`
	TraceId   string         `json:"traceId,omitempty"`
}

// Middleware recovers panics and converts errors raised via Abort/c.Error
// (or any panic) into the ErrorResponse body, using the request's OTel span
// as traceId. Register it early so it wraps every route.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logPanic(c, rec)
				writeError(c, New(http.StatusInternalServerError, "InternalError", "An unexpected error occurred."))
			}
		}()

		c.Next()

		if c.Writer.Written() || len(c.Errors) == 0 {
			return
		}
		writeError(c, toAPIError(c.Errors.Last().Err))
	}
}

func toAPIError(err error) *APIError {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr
	}
	return New(http.StatusInternalServerError, "InternalError", "An unexpected error occurred.")
}

func writeError(c *gin.Context, apiErr *APIError) {
	markError(c, apiErr)
	logError(c, apiErr)
	c.JSON(apiErr.Status, response{
		Error:     apiErr.Code,
		Message:   apiErr.Message,
		Details:   apiErr.Details,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TraceId:   traceID(c),
	})
	c.Abort()
}

// logError emits the error once so actual error text lands in the structured
// logs, correlated with the traceId returned to the client. 5xx errors are
// logged at error level, 4xx at warn.
func logError(c *gin.Context, apiErr *APIError) {
	ev := log.Log().Error()
	if apiErr.Status < 500 {
		ev = log.Log().Warn()
	}
	ev = ev.
		Str("http.method", c.Request.Method).
		Str("http.path", c.Request.URL.Path).
		Int("http.status_code", apiErr.Status).
		Str("error.code", apiErr.Code)
	if apiErr.Details != nil {
		ev = ev.Any("error.details", apiErr.Details)
	}
	if sc := trace.SpanContextFromContext(c.Request.Context()); sc.IsValid() {
		ev = ev.Str("trace_id", sc.TraceID().String()).Str("span_id", sc.SpanID().String())
	}
	ev.Msg(apiErr.Message)
}

// logPanic records a recovered panic so it is not silently swallowed.
func logPanic(c *gin.Context, rec any) {
	sc := trace.SpanContextFromContext(c.Request.Context())
	ev := log.Log().Error().
		Interface("panic", rec).
		Str("http.method", c.Request.Method).
		Str("http.path", c.Request.URL.Path).
		Int("http.status_code", http.StatusInternalServerError)
	if sc.IsValid() {
		ev = ev.Str("trace_id", sc.TraceID().String()).Str("span_id", sc.SpanID().String())
	}
	ev.Msg("panic recovered")
}

// markError records the error on the request span so the trace tree is
// consistent even when the error only surfaces at the middleware boundary.
// Only 5xx errors are treated as span errors; 4xx client errors are not.
func markError(c *gin.Context, apiErr *APIError) {
	if apiErr.Status < http.StatusInternalServerError {
		return
	}
	span := trace.SpanFromContext(c.Request.Context())
	span.RecordError(apiErr)
	span.SetStatus(codes.Error, apiErr.Message)
}

func traceID(c *gin.Context) string {
	sc := trace.SpanContextFromContext(c.Request.Context())
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}
