// Package apikitgin writes apikit envelopes, lists and objects through a
// gin.Context, binds list parameters from the query string, and carries the
// locale middleware. The envelope vocabulary itself lives in apikit, which
// depends on nothing.
package apikitgin

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/open-rails/apikit"
)

// Fail writes err as the canonical envelope, deriving status and shape from it.
func Fail(c *gin.Context, err error) {
	status, env := apikit.EnvelopeFor(err)
	c.JSON(status, env)
}

// Send writes an explicit envelope. Code may be empty: an absent code means
// "no machine reason beyond the status", and the writers never invent one.
func Send(c *gin.Context, status int, t apikit.Type, code apikit.Code, message, param string) {
	c.JSON(status, apikit.NewEnvelope(apikit.ErrorObject{Type: t, Code: code, Message: message, Param: param}))
}

func send(c *gin.Context, status int, code apikit.Code, message, param string) {
	Send(c, status, apikit.TypeForStatus(status), code, message, param)
}

// BadRequest sends 400 invalid_request_error.
func BadRequest(c *gin.Context, message string) {
	send(c, http.StatusBadRequest, "", message, "")
}

// BadRequestWithCode sends 400 with a machine-readable code.
func BadRequestWithCode(c *gin.Context, code apikit.Code, message string) {
	send(c, http.StatusBadRequest, code, message, "")
}

// BadRequestParam sends 400 naming the offending request field.
func BadRequestParam(c *gin.Context, param, message string) {
	send(c, http.StatusBadRequest, "", message, param)
}

// Unauthorized sends 401 authentication_error.
func Unauthorized(c *gin.Context) { UnauthorizedWithMessage(c, "unauthorized") }

// UnauthorizedWithMessage sends 401 with a custom message.
func UnauthorizedWithMessage(c *gin.Context, message string) {
	send(c, http.StatusUnauthorized, "", message, "")
}

// Forbidden sends 403 authorization_error.
func Forbidden(c *gin.Context) { ForbiddenWithMessage(c, "forbidden") }

// ForbiddenWithMessage sends 403 with a custom message.
func ForbiddenWithMessage(c *gin.Context, message string) {
	send(c, http.StatusForbidden, "", message, "")
}

// NotFound sends 404 not_found_error for a named entity.
func NotFound(c *gin.Context, entity string) {
	NotFoundWithMessage(c, fmt.Sprintf("%s not found", entity))
}

// NotFoundWithMessage sends 404 with a custom message.
func NotFoundWithMessage(c *gin.Context, message string) {
	send(c, http.StatusNotFound, "", message, "")
}

// Conflict sends 409 conflict_error.
func Conflict(c *gin.Context, message string) {
	send(c, http.StatusConflict, "", message, "")
}

// Rejected sends 422 rejected_error: a policy refused the content. It is the
// one 4xx category a status cannot imply, so it has its own writer.
func Rejected(c *gin.Context, reason string) {
	Send(c, http.StatusUnprocessableEntity, apikit.TypeRejected, apikit.CodeContentRejected, reason, "")
}

// UnprocessableEntity sends 422 invalid_request_error: well-formed syntax the
// server cannot act on. For a moderation refusal use Rejected.
func UnprocessableEntity(c *gin.Context, message string) {
	send(c, http.StatusUnprocessableEntity, "", message, "")
}

// UnsupportedMediaType sends 415 invalid_request_error.
func UnsupportedMediaType(c *gin.Context, message string) {
	send(c, http.StatusUnsupportedMediaType, "", message, "")
}

// TooManyRequests sends 429 rate_limit_error.
func TooManyRequests(c *gin.Context, message string) {
	send(c, http.StatusTooManyRequests, "", message, "")
}

// InternalError sends 500 api_error.
func InternalError(c *gin.Context, message string) {
	send(c, http.StatusInternalServerError, "", message, "")
}

// BadGateway sends 502 api_error for an upstream failure.
func BadGateway(c *gin.Context, message string) {
	send(c, http.StatusBadGateway, "", message, "")
}

// ServiceUnavailable sends 503 api_error.
func ServiceUnavailable(c *gin.Context, message string) {
	send(c, http.StatusServiceUnavailable, "", message, "")
}

// NotImplemented sends 501 not_implemented_error: this build lacks the capability.
func NotImplemented(c *gin.Context, message string) {
	send(c, http.StatusNotImplemented, "", message, "")
}

// NotConfigured sends 501 not_implemented_error for an optional capability the
// operator never wired — a deployment gap, distinguishable from a crash.
func NotConfigured(c *gin.Context, message string) {
	send(c, http.StatusNotImplemented, apikit.CodeNotConfigured, message, "")
}
