// Package apikit is the fleet's HTTP API envelope: one error shape, one type
// and code vocabulary, one status inference, and the list/pagination shapes
// that go with them. It depends on nothing outside the standard library, so
// every library in the fleet can speak it. Framework bindings live in
// sub-modules (adapters/gin).
package apikit

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Type is the transport-level category of a failure. The constants below are
// the standard set every service shares; Type is an open string so a domain
// may add its own (openrails' card_error is a legitimate extension, not drift).
//
// The rule for adding one: a Type exists when a client must take a DIFFERENT
// ACTION. Anything that only explains a failure belongs in Code.
type Type string

const (
	// TypeInvalidRequest: fix the input, then retry. 400 and unclassified 4xx.
	TypeInvalidRequest Type = "invalid_request_error"
	// TypeAuthentication: authenticate or refresh, then retry. 401.
	TypeAuthentication Type = "authentication_error"
	// TypeAuthorization: never retry as this principal. 403.
	TypeAuthorization Type = "authorization_error"
	// TypeNotFound: render an absent/gone state. 404.
	TypeNotFound Type = "not_found_error"
	// TypeConflict: reload current state, reconcile, retry. 409.
	TypeConflict Type = "conflict_error"
	// TypeRejected: a policy refused the content; show the reason, never retry
	// unchanged. 422, and never inferred — see TypeForStatus.
	TypeRejected Type = "rejected_error"
	// TypeRateLimit: back off and retry later. 429.
	TypeRateLimit Type = "rate_limit_error"
	// TypeNotImplemented: the capability is absent from this deployment; hide
	// the feature, never retry. 501.
	TypeNotImplemented Type = "not_implemented_error"
	// TypeAPI: the server failed; retry with backoff. 5xx other than 501.
	TypeAPI Type = "api_error"
)

// Code is a stable, machine-readable reason. The constants below are the
// transport-level codes shared across services; each library keeps its own
// domain catalog (authkit's hundreds of auth codes, openrails' decline codes)
// and emits those in the same field.
type Code string

const (
	CodeInvalidParam  Code = "invalid_param"
	CodeMissingParam  Code = "missing_param"
	CodeInvalidFormat Code = "invalid_format"

	CodeResourceNotFound Code = "resource_not_found"
	CodeResourceConflict Code = "resource_conflict"

	CodeAuthenticationRequired Code = "authentication_required"
	CodeInvalidToken           Code = "invalid_token"
	CodeTokenExpired           Code = "token_expired"
	CodeResourceAccessDenied   Code = "resource_access_denied"

	CodeRateLimitExceeded Code = "rate_limit_exceeded"

	// CodeContentRejected is a moderation/policy refusal of submitted content.
	CodeContentRejected Code = "content_rejected"
	// CodeNotConfigured is an optional capability the operator never wired —
	// a deployment gap, not a crash.
	CodeNotConfigured Code = "not_configured"
	// CodeNotImplemented is a capability this build does not have at all.
	CodeNotImplemented Code = "not_implemented"

	CodeInternalError      Code = "internal_error"
	CodeServiceUnavailable Code = "service_unavailable"
)

// ErrorObject is the error detail carried under the envelope's "error" key.
type ErrorObject struct {
	Type      Type           `json:"type"`
	Code      Code           `json:"code,omitempty"`
	Message   string         `json:"message"`
	Param     string         `json:"param,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Envelope is the whole error body: {"object":"error","error":{...}}.
// The object discriminator lets a client tell an error body from a success
// body without consulting the status.
type Envelope struct {
	Object string      `json:"object"` // always "error"
	Error  ErrorObject `json:"error"`
}

// NewEnvelope builds an envelope with the object discriminator set.
func NewEnvelope(obj ErrorObject) Envelope {
	if obj.Type == "" {
		obj.Type = TypeAPI
	}
	if len(obj.Metadata) == 0 {
		obj.Metadata = nil
	}
	return Envelope{Object: "error", Error: obj}
}

// TypeForStatus is the category an HTTP status determines on its own.
// It infers ONLY where the status is unambiguous: 422 could be a malformed
// entity or a policy rejection, so it yields TypeInvalidRequest and a caller
// that means TypeRejected must say so.
func TypeForStatus(status int) Type {
	switch status {
	case http.StatusUnauthorized:
		return TypeAuthentication
	case http.StatusForbidden:
		return TypeAuthorization
	case http.StatusNotFound:
		return TypeNotFound
	case http.StatusConflict:
		return TypeConflict
	case http.StatusTooManyRequests:
		return TypeRateLimit
	case http.StatusNotImplemented:
		return TypeNotImplemented
	}
	if status >= 500 {
		return TypeAPI
	}
	return TypeInvalidRequest
}

// CodeForStatus is the default code for a status. Writers do NOT apply it:
// an absent code means "no machine reason beyond the status", and filling one
// in silently would change what clients that prefer code over message display.
// Call it explicitly when you want a code on every error.
func CodeForStatus(status int) Code {
	switch status {
	case http.StatusUnauthorized:
		return CodeAuthenticationRequired
	case http.StatusForbidden:
		return CodeResourceAccessDenied
	case http.StatusNotFound:
		return CodeResourceNotFound
	case http.StatusConflict:
		return CodeResourceConflict
	case http.StatusTooManyRequests:
		return CodeRateLimitExceeded
	case http.StatusNotImplemented:
		return CodeNotImplemented
	case http.StatusServiceUnavailable:
		return CodeServiceUnavailable
	}
	if status >= 500 {
		return CodeInternalError
	}
	return CodeInvalidParam
}

// Error is the transport carrier: a library maps its own sentinel onto one of
// these and a single writer renders it. It is not a catalog — libraries keep
// their own.
type Error struct {
	Status    int
	Type      Type
	Code      Code
	Message   string
	Param     string
	RequestID string
	Metadata  map[string]any

	cause error
}

// E builds an Error, inferring the type from the status when none is given.
func E(status int, code Code, message string) *Error {
	return &Error{Status: status, Type: TypeForStatus(status), Code: code, Message: message}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.Message + ": " + e.cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.cause }

// Is matches any *Error carrying the same Code, so a sentinel, a fresh E() and
// a wrapped copy are one identity.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code != "" && t.Code == e.Code
}

func (e *Error) WithCause(cause error) *Error   { e.cause = cause; return e }
func (e *Error) WithParam(param string) *Error  { e.Param = param; return e }
func (e *Error) WithType(t Type) *Error         { e.Type = t; return e }
func (e *Error) WithRequestID(id string) *Error { e.RequestID = id; return e }
func (e *Error) WithMetadata(m map[string]any) *Error {
	if e.Metadata == nil {
		e.Metadata = map[string]any{}
	}
	for k, v := range m {
		e.Metadata[k] = v
	}
	return e
}

// Envelope renders the error as its wire body.
func (e *Error) Envelope() Envelope {
	return NewEnvelope(ErrorObject{
		Type:      e.Type,
		Code:      e.Code,
		Message:   e.Message,
		Param:     e.Param,
		RequestID: e.RequestID,
		Metadata:  e.Metadata,
	})
}

// AsError returns the *Error in err's chain, or nil.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// EnvelopeFor derives the wire status and envelope for any error. An error
// that is not an *Error — and any 5xx — is rendered as a bare internal error,
// so an internal message never reaches the wire.
func EnvelopeFor(err error) (int, Envelope) {
	e := AsError(err)
	if e == nil {
		return http.StatusInternalServerError, NewEnvelope(ErrorObject{Type: TypeAPI, Code: CodeInternalError, Message: "internal error"})
	}
	status := e.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	// 500 alone is scrubbed: it is the one status that means "unexpected", so
	// its message may carry an internal cause. 501/502/503 state a deliberate
	// operational condition and keep theirs.
	if status == http.StatusInternalServerError {
		return status, NewEnvelope(ErrorObject{Type: TypeAPI, Code: CodeInternalError, Message: "internal error", RequestID: e.RequestID})
	}
	env := e.Envelope()
	if env.Error.Type == "" {
		env.Error.Type = TypeForStatus(status)
	}
	return status, env
}

// WriteError writes err as the canonical envelope over net/http.
func WriteError(w http.ResponseWriter, err error) {
	status, env := EnvelopeFor(err)
	WriteJSON(w, status, env)
}

// WriteJSON writes v as a JSON body with the canonical content type.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
