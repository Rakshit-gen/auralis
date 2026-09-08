// Package errcodes defines the platform-wide error envelope and machine-readable
// error codes. Every service returns the same JSON shape on failure:
//
//	{"error":{"code":"content.show_not_found","message":"show not found","request_id":"..."}}
//
// Stack traces and internal detail are never placed in the response.
package errcodes

import (
	"encoding/json"
	"net/http"
)

// Code is a stable, machine-readable identifier of the form "<domain>.<reason>".
type Code string

const (
	// Cross-cutting codes shared by every service.
	Unauthorized   Code = "auth.unauthorized"
	Forbidden      Code = "auth.forbidden"
	InvalidToken   Code = "auth.invalid_token"
	Validation     Code = "request.validation_failed"
	NotFound       Code = "request.not_found"
	Conflict       Code = "request.conflict"
	RateLimited    Code = "request.rate_limited"
	PayloadTooBig  Code = "request.payload_too_large"
	Unavailable    Code = "dependency.unavailable"
	Timeout        Code = "dependency.timeout"
	Internal       Code = "internal.unexpected"
	NotImplemented Code = "internal.not_implemented"
)

// Error is an application error carrying an HTTP status and a stable code.
type Error struct {
	Status  int
	Code    Code
	Message string
	// Fields optionally lists per-field validation problems.
	Fields map[string]string
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// New builds an *Error.
func New(status int, code Code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Common constructors.
func Unauthed(msg string) *Error    { return New(http.StatusUnauthorized, Unauthorized, msg) }
func Denied(msg string) *Error      { return New(http.StatusForbidden, Forbidden, msg) }
func BadRequest(msg string) *Error  { return New(http.StatusBadRequest, Validation, msg) }
func Missing(msg string) *Error     { return New(http.StatusNotFound, NotFound, msg) }
func Conflicting(msg string) *Error { return New(http.StatusConflict, Conflict, msg) }
func Unexpected(msg string) *Error  { return New(http.StatusInternalServerError, Internal, msg) }

// WithFields attaches field-level validation detail.
func (e *Error) WithFields(f map[string]string) *Error { e.Fields = f; return e }

type body struct {
	Error payload `json:"error"`
}

type payload struct {
	Code      Code              `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// Write serialises err to w using the platform error envelope. Non-*Error values
// are reported as internal.unexpected without leaking their message.
func Write(w http.ResponseWriter, requestID string, err error) {
	e, ok := err.(*Error)
	if !ok {
		e = Unexpected("an unexpected error occurred")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(body{Error: payload{
		Code:      e.Code,
		Message:   e.Message,
		RequestID: requestID,
		Fields:    e.Fields,
	}})
}
