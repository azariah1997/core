// Package apperr defines the platform's single consistent error model so
// every service returns the same {code, message, correlationId} shape and
// never leaks stack traces to clients.
package apperr

import (
	"net/http"

	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

type Code string

const (
	CodeValidation      Code = "VALIDATION_ERROR"
	CodeUnauthenticated Code = "AUTHENTICATION_REQUIRED"
	CodeAccessDenied    Code = "ACCESS_DENIED"
	CodeNotFound        Code = "RESOURCE_NOT_FOUND"
	CodeConflict        Code = "CONFLICT"
	CodeRateLimited     Code = "RATE_LIMITED"
	CodeInternal        Code = "INTERNAL_ERROR"
	CodeDependency      Code = "DEPENDENCY_FAILURE"
	CodeNotImplemented  Code = "NOT_IMPLEMENTED"
)

var statusByCode = map[Code]int{
	CodeValidation:      http.StatusBadRequest,
	CodeUnauthenticated: http.StatusUnauthorized,
	CodeAccessDenied:    http.StatusForbidden,
	CodeNotFound:        http.StatusNotFound,
	CodeConflict:        http.StatusConflict,
	CodeRateLimited:     http.StatusTooManyRequests,
	CodeInternal:        http.StatusInternalServerError,
	CodeDependency:      http.StatusBadGateway,
	CodeNotImplemented:  http.StatusNotImplemented,
}

// Error is the platform's standard application error.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// New creates an Error with the given code and message.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

type response struct {
	Code          Code   `json:"code"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlationId,omitempty"`
}

// Write sends err as the standard JSON error envelope, mapping its Code to
// the matching HTTP status and attaching the request's correlation ID.
func Write(w http.ResponseWriter, r *http.Request, err *Error) {
	status, ok := statusByCode[err.Code]
	if !ok {
		status = http.StatusInternalServerError
	}
	httpx.JSON(w, status, response{
		Code:          err.Code,
		Message:       err.Message,
		CorrelationID: correlation.FromContext(r.Context()),
	})
}
