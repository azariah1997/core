package coresdk

import "fmt"

// APIError mirrors the platform's real error envelope
// (packages/go/platformkit/apperr.Error's wire format: {code, message,
// correlationId}) - every non-2xx response from core-api decodes into
// one of these, never a generic "request failed" string.
type APIError struct {
	StatusCode    int    `json:"-"`
	Code          string `json:"code"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlationId,omitempty"`
}

func (e *APIError) Error() string {
	if e.CorrelationID != "" {
		return fmt.Sprintf("coresdk: %s: %s (status %d, correlation %s)", e.Code, e.Message, e.StatusCode, e.CorrelationID)
	}
	return fmt.Sprintf("coresdk: %s: %s (status %d)", e.Code, e.Message, e.StatusCode)
}

// Known apperr.Code values, duplicated here (not imported from
// platformkit/apperr) deliberately - a real product consuming this SDK
// should never need to import a server-internal package to check an
// error code.
const (
	CodeValidation      = "VALIDATION_ERROR"
	CodeUnauthenticated = "AUTHENTICATION_REQUIRED"
	CodeAccessDenied    = "ACCESS_DENIED"
	CodeNotFound        = "RESOURCE_NOT_FOUND"
	CodeConflict        = "CONFLICT"
	CodeRateLimited     = "RATE_LIMITED"
	CodeInternal        = "INTERNAL_ERROR"
	CodeDependency      = "DEPENDENCY_FAILURE"
	CodeNotImplemented  = "NOT_IMPLEMENTED"
)

// IsCode reports whether err is an *APIError with the given code - the
// idiomatic way calling code should branch on a specific failure
// (e.g. `if coresdk.IsCode(err, coresdk.CodeConflict) { ... }`) instead
// of comparing status codes or parsing the error string.
func IsCode(err error, code string) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.Code == code
}
