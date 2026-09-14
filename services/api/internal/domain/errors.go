// Package domain holds the entities and the rules that govern them.
//
// Nothing here imports net/http, pgx or any other piece of infrastructure,
// and that constraint is the point: these rules are the ones that must hold
// whoever is asking, whether that is an HTTP handler, the expiry sweeper
// goroutine or a future CLI. If this package ever needs to know how it is
// being called, a rule has leaked into the wrong layer.
package domain

import (
	"errors"
	"fmt"
)

// Kind classifies an error by what the caller ought to do about it.
//
// It deliberately carries no HTTP status. The domain does not know it is being
// served over HTTP, and a use case driven by a background worker has no status
// code to return. Translating a Kind into a status is the HTTP adapter's job,
// which keeps the mapping in exactly one place.
type Kind uint8

const (
	// KindInternal is the zero value on purpose: an error that nobody
	// classified is a bug, and defaulting to 500 is the safe failure. The
	// alternative default, treating unknown errors as client errors, hides
	// server faults behind a 400 and they stop being alerted on.
	KindInternal Kind = iota

	// KindInvalid means the input was rejected by a business rule.
	KindInvalid

	// KindUnauthenticated means the caller has not proved who they are.
	KindUnauthenticated

	// KindForbidden means the caller is known but not allowed. Distinct from
	// KindUnauthenticated because retrying with fresh credentials will not
	// help.
	KindForbidden

	// KindNotFound means the entity does not exist.
	KindNotFound

	// KindConflict means the request clashes with the current state, such as
	// claiming a spot somebody else just took.
	KindConflict
)

// Error is a domain failure with enough structure for an adapter to render it
// without a switch over every individual error value.
type Error struct {
	Kind Kind

	// Code is a stable, machine-readable identifier. Clients branch on this,
	// so it is part of the API contract and must not be reworded casually;
	// Message may change freely.
	Code string

	// Message is safe to show a user. It must never carry internal detail:
	// everything in here reaches the client.
	Message string

	// Fields maps an input name to what is wrong with it, for validation
	// failures. A client can then highlight the offending form field instead
	// of showing one lump of prose.
	Fields map[string]string

	// cause is the underlying error, kept for logs and never serialised.
	cause error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap exposes the cause so errors.Is and errors.As keep working through a
// domain error.
func (e *Error) Unwrap() error { return e.cause }

// KindOf reports how an error should be treated, walking the wrap chain.
//
// An unclassified error is internal, which is the conservative answer: a fault
// nobody thought about should page somebody, not be quietly reported to the
// user as their own mistake.
func KindOf(err error) Kind {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Kind
	}
	return KindInternal
}

// AsError returns the domain error in the chain, if there is one.
func AsError(err error) (*Error, bool) {
	var domainErr *Error
	ok := errors.As(err, &domainErr)
	return domainErr, ok
}

// Invalid reports input rejected by a rule.
func Invalid(code, message string) *Error {
	return &Error{Kind: KindInvalid, Code: code, Message: message}
}

// InvalidFields reports per-field validation failures.
func InvalidFields(fields map[string]string) *Error {
	return &Error{
		Kind:    KindInvalid,
		Code:    "validation_failed",
		Message: "some fields are invalid",
		Fields:  fields,
	}
}

// Unauthenticated reports missing or unusable credentials.
func Unauthenticated(code, message string) *Error {
	return &Error{Kind: KindUnauthenticated, Code: code, Message: message}
}

// Forbidden reports an authenticated caller acting outside their rights.
func Forbidden(code, message string) *Error {
	return &Error{Kind: KindForbidden, Code: code, Message: message}
}

// NotFound reports a missing entity.
func NotFound(code, message string) *Error {
	return &Error{Kind: KindNotFound, Code: code, Message: message}
}

// Conflict reports a clash with current state.
func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

// Internal wraps a fault that is not the caller's fault.
//
// The message is fixed and generic because this is the one case where the
// cause must not reach the client: it is where driver errors and SQL text
// live, and those describe the inside of the system.
func Internal(cause error) *Error {
	return &Error{
		Kind:    KindInternal,
		Code:    "internal",
		Message: "something went wrong on our side",
		cause:   cause,
	}
}

// Wrap attaches a cause to a domain error, for logging.
func (e *Error) Wrap(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

// Errors the adapters raise and the services interpret.
//
// These are plumbing-level sentinels rather than user-facing failures: a port
// reports them and the service decides what they mean for the use case. For
// example a missing row is ErrNoRows to the adapter, but whether that is a 404
// or a deliberately vague 401 is a decision only the use case can make.
var (
	// ErrNoRows means a query matched nothing.
	ErrNoRows = errors.New("domain: no rows")

	// ErrDuplicate means a uniqueness constraint rejected the write.
	ErrDuplicate = errors.New("domain: duplicate key")

	// ErrTokenReused means a refresh token that had already been rotated was
	// presented a second time. A legitimate client uses each token exactly
	// once, so this happens only if one leaked.
	ErrTokenReused = errors.New("domain: refresh token was already used")

	// ErrTokenExpired means a stored token is past its lifetime.
	ErrTokenExpired = errors.New("domain: token has expired")

	// ErrConflict means a conditional write matched no rows because the
	// entity had already moved on. It is how an adapter reports that somebody
	// else won a race.
	ErrConflict = errors.New("domain: conflicting state")

	// ErrInsufficientFunds means a hold would take the balance below zero.
	ErrInsufficientFunds = errors.New("domain: insufficient funds")

	// ErrOwnResource means the caller tried to claim something they own.
	ErrOwnResource = errors.New("domain: acting on own resource")
)
