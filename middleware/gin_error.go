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
	"go.opentelemetry.io/otel/trace"
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
	c.JSON(apiErr.Status, response{
		Error:     apiErr.Code,
		Message:   apiErr.Message,
		Details:   apiErr.Details,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TraceId:   traceID(c),
	})
	c.Abort()
}

func traceID(c *gin.Context) string {
	sc := trace.SpanContextFromContext(c.Request.Context())
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}
