// Package web holds the HTTP plumbing shared by every handler: the response
// helpers, the error envelope, the middleware chain and the request context
// values.
package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// Handler is an http.Handler whose implementation may return an error instead
// of writing a failure response itself.
//
// This is the whole reason handlers in this codebase read as a sequence of
// "do the thing or return why not": without it every handler ends up with four
// copies of "log, set status, encode envelope, return".
type Handler func(http.ResponseWriter, *http.Request) error

// ServeHTTP makes Handler satisfy http.Handler.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		WriteError(w, r, err)
	}
}

// JSON writes v as the response body with the given status.
//
// The body is encoded into memory first. Encoding straight to the
// ResponseWriter would commit a 200 status before discovering that v is
// unserialisable, leaving a truncated body behind a success code.
func JSON(w http.ResponseWriter, status int, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return Internal(err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)

	// A failed write means the client vanished. There is nobody left to tell,
	// and the logging middleware already records the request.
	_, _ = w.Write(body)
	return nil
}

// NoContent replies 204 with an empty body.
func NoContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
}

// WriteError serialises err into the uniform error envelope.
//
// Server-side faults are logged at error level with their cause; client
// mistakes are logged at debug level, because a wall of warnings for ordinary
// 404s trains everyone to ignore the logs.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := asError(err)
	requestID := RequestIDFrom(r.Context())
	log := LoggerFrom(r.Context())

	attrs := []any{
		slog.String("code", apiErr.Code),
		slog.Int("status", apiErr.Status),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
	}

	if apiErr.Status >= http.StatusInternalServerError {
		if cause := apiErr.Unwrap(); cause != nil {
			attrs = append(attrs, slog.Any("cause", cause))
		}
		log.Error("request failed", attrs...)
	} else {
		log.Debug("request rejected", attrs...)
	}

	if apiErr.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(apiErr.RetryAfter))
	}

	body := errorEnvelope{Error: errorBody{
		Code:      apiErr.Code,
		Message:   apiErr.Message,
		Fields:    apiErr.Fields,
		RequestID: requestID,
	}}

	encoded, marshalErr := json.Marshal(body)
	if marshalErr != nil {
		// The envelope is a fixed shape of strings, so this cannot realistically
		// fail; if it somehow does, a bare status is better than a panic.
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
	w.WriteHeader(apiErr.Status)
	_, _ = w.Write(encoded)
}

// DecodeJSON reads a JSON request body into dst.
//
// It rejects unknown fields so that a client misspelling "price_cents" is told
// so, rather than silently having its value ignored, and it caps the body size
// so an oversized payload cannot exhaust memory.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	const maxBodyBytes = 1 << 20 // 1 MiB

	if ct := r.Header.Get("Content-Type"); ct != "" &&
		ct != "application/json" && !hasJSONPrefix(ct) {
		return BadRequest("unsupported_media_type", "Content-Type must be application/json")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return BadRequest("invalid_json", "request body is not valid JSON: "+err.Error())
	}

	// A second value in the stream means the client sent something like
	// `{} {}`, which is never intentional.
	if decoder.More() {
		return BadRequest("invalid_json", "request body must contain a single JSON object")
	}

	return nil
}

func hasJSONPrefix(contentType string) bool {
	const prefix = "application/json;"
	return len(contentType) >= len(prefix) && contentType[:len(prefix)] == prefix
}
