package compat

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/open-rails/apikit"
	apikitgin "github.com/open-rails/apikit/adapters/gin"
)

// TestAPIKitCanonical records what THIS module emits, beside the three
// deployed writers, so the revision can be compared to what it replaces.
func TestAPIKitCanonical(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"400-bad-request", apikit.E(http.StatusBadRequest, "", "amount is required")},
		{"400-with-code", apikit.E(http.StatusBadRequest, apikit.CodeMissingParam, "amount is required")},
		{"400-with-param", apikit.E(http.StatusBadRequest, "", "amount is required").WithParam("amount")},
		{"401-unauthorized", apikit.E(http.StatusUnauthorized, "", "unauthorized")},
		{"402-card", apikit.E(http.StatusPaymentRequired, "insufficient_funds", "card declined")},
		{"403-forbidden", apikit.E(http.StatusForbidden, "", "forbidden")},
		{"404-not-found", apikit.E(http.StatusNotFound, "", "gallery not found")},
		{"409-conflict", apikit.E(http.StatusConflict, "", "already exists")},
		{"415-unsupported-media", apikit.E(http.StatusUnsupportedMediaType, "", "unsupported media type")},
		{"422-moderation-rejected", apikit.E(http.StatusUnprocessableEntity, apikit.CodeModerationRejected, "the artwork was refused")},
		{"429-rate-limit", apikit.E(http.StatusTooManyRequests, "", "slow down").WithMetadata(map[string]any{"retry_after": 30})},
		{"500-internal", apikit.E(http.StatusInternalServerError, "", `pq: constraint "galleries_slug_key" violated`)},
		{"501-not-implemented", apikit.E(http.StatusNotImplemented, apikit.CodeNotImplemented, "video transcoding is not configured")},
		{"502-bad-gateway", apikit.E(http.StatusBadGateway, "", "upstream failed")},
		{"503-unavailable", apikit.E(http.StatusServiceUnavailable, "", "try later")},
		{"with-request-id", apikit.E(http.StatusConflict, apikit.CodeResourceConflict, "idempotency key reused").WithRequestID("req_01HZX")},
		// The leak that must not reach the wire, recorded as bytes.
		{"metadata-scrubbed", apikit.E(http.StatusConflict, apikit.CodeResourceConflict, "conflict").
			WithMetadata(map[string]any{"constraint": `pq: duplicate key value violates unique constraint "users_email_key"`, "retry_after": 5})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := netHTTP(func(w http.ResponseWriter, _ *http.Request) { apikit.WriteError(w, tc.err) })
			golden(t, "apikit/"+tc.name, capture(t, h, nil))
		})
	}
}

// TestAPIKitCompatObject records the SAME errors in the GinAPI-compatible
// shape. Two directories, one difference: the top-level discriminator. Feed
// both to spa/probe.mjs and the consequence of the open decision is data.
func TestAPIKitCompatObject(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"400-bad-request", apikit.E(http.StatusBadRequest, "", "amount is required")},
		{"404-not-found", apikit.E(http.StatusNotFound, "", "gallery not found")},
		{"422-moderation-rejected", apikit.E(http.StatusUnprocessableEntity, apikit.CodeModerationRejected, "the artwork was refused")},
		{"500-internal", apikit.E(http.StatusInternalServerError, "", "boom")},
		{"501-not-implemented", apikit.E(http.StatusNotImplemented, apikit.CodeNotImplemented, "video transcoding is not configured")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := netHTTP(func(w http.ResponseWriter, _ *http.Request) {
				status, env := apikit.EnvelopeFor(tc.err)
				apikit.WriteJSON(w, status, apikit.WithObject(env))
			})
			golden(t, "apikit-compat-object/"+tc.name, capture(t, h, nil))
		})
	}
}

// TestAPIKitGinMatchesNetHTTP is the adapter's cross-module guarantee, asserted
// here as well because this module is the one that reads bytes off a socket for
// every writer in the fleet.
func TestAPIKitGinMatchesNetHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	statuses := []int{400, 401, 403, 404, 409, 415, 422, 429, 500, 501, 502, 503}
	for _, status := range statuses {
		t.Run(fmt.Sprintf("%d", status), func(t *testing.T) {
			err := apikit.E(status, "", "probe message")
			r := gin.New()
			r.GET("/probe", func(c *gin.Context) { apikitgin.Fail(c, err) })
			viaGin := capture(t, r, nil)
			viaHTTP := capture(t, netHTTP(func(w http.ResponseWriter, _ *http.Request) { apikit.WriteError(w, err) }), nil)
			if viaGin != viaHTTP {
				t.Errorf("gin and net/http disagree at %d\n--- gin ---\n%s\n--- net/http ---\n%s", status, viaGin, viaHTTP)
			}
		})
	}
}
