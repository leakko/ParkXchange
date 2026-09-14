package web

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

// MethodNotAllowed reports a path that exists but does not accept this method.
func MethodNotAllowed() *Error {
	return &Error{
		Status:  http.StatusMethodNotAllowed,
		Code:    "method_not_allowed",
		Message: "that method is not supported for this path",
	}
}

// NormalizeErrors rewrites the plain-text failures net/http produces on its own
// into the API's JSON error envelope.
//
// ServeMux answers an unrouted path with "404 page not found" and a method
// mismatch with "405 method not allowed", both as text/plain, and
// MaxBytesReader does something similar. A client would then have to parse two
// different error formats and guess which one it got, so every failure is
// converted to the single documented shape here.
//
// Responses that handlers write themselves are left untouched, because they
// are already in the right format.
func NormalizeErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		interceptor := &errorNormalizer{ResponseWriter: w}
		next.ServeHTTP(interceptor, r)
		interceptor.flush(r)
	})
}

// plainTextContentType is exactly what net/http's http.Error sets, which is
// how we recognise a response it generated rather than one of ours.
const plainTextContentType = "text/plain; charset=utf-8"

type errorNormalizer struct {
	http.ResponseWriter

	wroteHeader bool
	capturing   bool
	status      int
}

func (n *errorNormalizer) WriteHeader(status int) {
	if n.wroteHeader {
		return
	}
	n.wroteHeader = true
	n.status = status

	if status >= http.StatusBadRequest &&
		n.Header().Get("Content-Type") == plainTextContentType {
		// Hold the response back so flush can replace it. Headers set by the
		// generator, such as Allow on a 405, are still in n.Header() and are
		// preserved.
		n.capturing = true
		return
	}

	n.ResponseWriter.WriteHeader(status)
}

func (n *errorNormalizer) Write(b []byte) (int, error) {
	if !n.wroteHeader {
		n.WriteHeader(http.StatusOK)
	}
	if n.capturing {
		// Swallow the plain-text body; flush writes the envelope instead.
		return len(b), nil
	}
	return n.ResponseWriter.Write(b)
}

func (n *errorNormalizer) flush(r *http.Request) {
	if !n.capturing {
		return
	}
	n.capturing = false

	// The generator's body framing no longer applies to what we are about to
	// write.
	n.Header().Del("Content-Type")
	n.Header().Del("Content-Length")

	var err *Error
	switch n.status {
	case http.StatusNotFound:
		err = NotFound("that resource")
	case http.StatusMethodNotAllowed:
		err = MethodNotAllowed()
	case http.StatusRequestEntityTooLarge:
		err = BadRequest("payload_too_large", "the request body is too large")
	default:
		err = &Error{
			Status:  n.status,
			Code:    "request_rejected",
			Message: http.StatusText(n.status),
		}
	}

	WriteError(n.ResponseWriter, r, err)
}

// Unwrap keeps http.ResponseController working through this wrapper.
func (n *errorNormalizer) Unwrap() http.ResponseWriter { return n.ResponseWriter }

// Hijack is required for the WebSocket upgrade to survive the chain.
func (n *errorNormalizer) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := n.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("web: %T does not support hijacking", n.ResponseWriter)
	}
	return hijacker.Hijack()
}

func (n *errorNormalizer) Flush() {
	if flusher, ok := n.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
