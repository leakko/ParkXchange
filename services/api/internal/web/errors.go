package web

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is an HTTP-aware error. Handlers return it to describe a failure the
// client is allowed to see; anything else they return is treated as an
// internal fault and reported as a bare 500.
//
// Code is a stable machine-readable string. Clients branch on it, so it is
// part of the API contract and must not be reworded casually. Message is for
// humans and may change freely.
type Error struct {
	Status  int
	Code    string
	Message string

	// Fields maps a request field to what is wrong with it, for validation
	// failures.
	Fields map[string]string

	// RetryAfter populates the Retry-After header when non-zero (seconds).
	RetryAfter int

	// cause is logged but never serialised: internal detail must not leak to
	// a client, and a stack of wrapped driver errors is no use to them anyway.
	cause error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap exposes the cause to errors.Is and errors.As without exposing it to
// the client.
func (e *Error) Unwrap() error { return e.cause }

// WithCause attaches an underlying error for the logs.
func (e *Error) WithCause(cause error) *Error {
	e.cause = cause
	return e
}

// BadRequest reports a malformed or semantically invalid request.
func BadRequest(code, message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: code, Message: message}
}

// InvalidFields reports a validation failure, naming each offending field.
func InvalidFields(fields map[string]string) *Error {
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    "validation_failed",
		Message: "one or more fields are invalid",
		Fields:  fields,
	}
}

// Unauthorized reports missing or unusable credentials.
func Unauthorized(message string) *Error {
	return &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: message}
}

// Forbidden reports valid credentials that do not permit this action.
func Forbidden(message string) *Error {
	return &Error{Status: http.StatusForbidden, Code: "forbidden", Message: message}
}

// NotFound reports a missing resource.
//
// It is also the correct answer for a resource that exists but does not belong
// to the caller: replying 403 would confirm the identifier is real, which lets
// an attacker enumerate other people's spots.
func NotFound(resource string) *Error {
	return &Error{
		Status:  http.StatusNotFound,
		Code:    "not_found",
		Message: resource + " was not found",
	}
}

// Conflict reports a request that lost a race or contradicts current state,
// such as claiming a spot somebody else just took.
func Conflict(code, message string) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: message}
}

// TooManyRequests reports that the caller exceeded its rate limit.
func TooManyRequests(retryAfterSeconds int) *Error {
	return &Error{
		Status:     http.StatusTooManyRequests,
		Code:       "rate_limited",
		Message:    "too many requests, slow down",
		RetryAfter: retryAfterSeconds,
	}
}

// Internal wraps an unexpected failure. The cause reaches the logs; the client
// gets nothing but the code.
func Internal(cause error) *Error {
	return &Error{
		Status:  http.StatusInternalServerError,
		Code:    "internal_error",
		Message: "something went wrong on our side",
		cause:   cause,
	}
}

// asError converts any error into an *Error, mapping anything unrecognised to
// a 500. This is what guarantees a handler cannot accidentally leak a driver
// error message to a client by returning it directly.
func asError(err error) *Error {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return Internal(err)
}
