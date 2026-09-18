// Package compat records, byte for byte, what every error writer already
// deployed in the fleet puts on the wire. It is evidence, not a library: no
// consumer imports it, and it is the only module here allowed to depend on
// authkit, openrails and ginapi.
//
// Every fixture is read off a real socket (httptest.NewServer + http.Get), so
// it captures header order, status, trailing newline and field order as a
// client sees them — not a struct round-trip.
//
// Regenerate: go test ./... -update
package compat

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	ginapi "github.com/doujins-org/ginapi/response"
	"github.com/gin-gonic/gin"
	"github.com/open-rails/authkit"
	openrailsapi "github.com/open-rails/openrails/pkg/api"
	"github.com/open-rails/openrails/pkg/billingauth"
)

var update = flag.Bool("update", false, "rewrite the golden fixtures")

// capture drives handler over a real socket and renders the response the way a
// client receives it.
func capture(t *testing.T, handler http.Handler, reqHeaders map[string]string) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/probe", nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "HTTP/1.1 %s\n", resp.Status)
	names := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		// Date and Content-Length vary per run; they are not part of the contract.
		if k == "Date" || k == "Content-Length" {
			continue
		}
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(&b, "%s: %s\n", k, strings.Join(resp.Header[k], ", "))
	}
	b.WriteString("\n")
	b.Write(body)
	return b.String()
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("golden", name+".http")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing fixture %s (run: go test ./... -update): %v", path, err)
	}
	if got != string(want) {
		t.Errorf("%s drifted.\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

func netHTTP(fn func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(fn)
}

// ---------------------------------------------------------------- authkit ---

// TestAuthKit records authkit v0.103.1's httperror.go on the wire. Its writer
// is authkit.WriteError; the envelope is nested with NO top-level object, code
// is ALWAYS present (no omitempty), param is *string+omitempty.
func TestAuthKit(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		// 400 with a catalog param: the catalog fills param when the site did not.
		{"400-invalid-request", authkit.E(authkit.CodeInvalidEmail)},
		{"400-explicit-param", authkit.E(authkit.CodeInvalidEmail, authkit.WithParam("email_address"))},
		{"401-authentication", authkit.E(authkit.CodeAuthenticationRequired)},
		{"403-authorization", authkit.E(authkit.CodeTwoFARequired)},
		// 404: authkit maps it to invalid_request_error, NOT a not_found type.
		{"404-not-found", authkit.E(authkit.CodeUserNotFound)},
		// 409: likewise invalid_request_error.
		{"409-conflict", authkit.E(authkit.CodeTwoFAFactorExists)},
		{"429-rate-limit", authkit.E(authkit.CodeRateLimited)},
		{"500-internal", authkit.E(authkit.CodeChallengeFailed)},
		{"500-unknown-error", fmt.Errorf("pq: duplicate key value violates unique constraint \"users_email_key\"")},
		{"502-upstream", authkit.E(authkit.CodeApplicationDocumentFetchFailed)},
		{"with-metadata", authkit.E(authkit.CodeRateLimited, authkit.WithMetadata(map[string]any{"retry_after": 30}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := netHTTP(func(w http.ResponseWriter, _ *http.Request) { authkit.WriteError(w, tc.err) })
			golden(t, "authkit/"+tc.name, capture(t, h, nil))
		})
	}
}

// -------------------------------------------------------------- openrails ---

// TestOpenRailsSimple records pkg/api.SimpleErrorResponse as the deployed
// net/http writers emit it (adminconsole.writeError, internal/http/request).
func TestOpenRailsSimple(t *testing.T) {
	statuses := []int{400, 401, 402, 403, 404, 409, 415, 422, 429, 500, 501, 502, 503}
	for _, status := range statuses {
		t.Run(fmt.Sprintf("%d", status), func(t *testing.T) {
			h := netHTTP(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(openrailsapi.SimpleErrorResponse(status, "probe message"))
			})
			golden(t, fmt.Sprintf("openrails/simple-%d", status), capture(t, h, nil))
		})
	}
}

// TestOpenRailsAPIError records APIError.ToResponse(): the full shape with
// request_id, param and metadata.
func TestOpenRailsAPIError(t *testing.T) {
	cases := []struct {
		name string
		err  *openrailsapi.APIError
	}{
		{"plain", openrailsapi.NewAPIError(400, openrailsapi.ErrorTypeInvalidRequest, openrailsapi.CodeInvalidParam, "amount is required")},
		{"with-request-id", openrailsapi.NewAPIError(409, openrailsapi.ErrorTypeInvalidRequest, openrailsapi.CodeIdempotencyKeyReused, "idempotency key reused").WithRequestID("req_01HZX")},
		{"with-param", openrailsapi.NewAPIError(400, openrailsapi.ErrorTypeInvalidRequest, openrailsapi.CodeInvalidParam, "amount is required").WithParam("amount")},
		{"card-error", openrailsapi.NewAPIError(402, openrailsapi.ErrorTypeCard, openrailsapi.CodeInsufficientFunds, "insufficient funds").WithRequestID("req_01HZY")},
		{"with-metadata", openrailsapi.NewAPIError(429, openrailsapi.ErrorTypeRateLimit, openrailsapi.CodeRateLimitExceeded, "slow down").WithMetadata(map[string]any{"retry_after": 30})},
		{"conflict-helper", openrailsapi.ConflictError("payment already captured")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := netHTTP(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.err.HTTPStatus)
				_ = json.NewEncoder(w).Encode(tc.err.ToResponse())
			})
			golden(t, "openrails/apierror-"+tc.name, capture(t, h, nil))
		})
	}
}

// TestOpenRailsBillingAuth records pkg/billingauth.WriteJSONError, the one
// exported real-socket writer: it echoes X-Request-ID into request_id.
func TestOpenRailsBillingAuth(t *testing.T) {
	for _, withID := range []bool{false, true} {
		name := "no-request-id"
		if withID {
			name = "with-request-id"
		}
		t.Run(name, func(t *testing.T) {
			h := netHTTP(func(w http.ResponseWriter, _ *http.Request) {
				if withID {
					w.Header().Set("X-Request-ID", "req_01HZZ")
				}
				billingauth.WriteJSONError(w, http.StatusUnauthorized, "authentication_required", "authentication required")
			})
			golden(t, "openrails/billingauth-"+name, capture(t, h, nil))
		})
	}
}

// ----------------------------------------------------------------- ginapi ---

func ginServer(fn func(c *gin.Context)) http.Handler {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/probe", fn)
	return r
}

// TestGinAPI records doujins-org/ginapi v0.5.0 (= hentai0's pin) on the wire.
// TestGinAPIVersionsAgree proves v0.4.0 (= doujins' pin) is byte-identical.
func TestGinAPI(t *testing.T) {
	cases := []struct {
		name string
		fn   func(c *gin.Context)
	}{
		{"400-bad-request", func(c *gin.Context) { ginapi.BadRequest(c, "amount is required") }},
		{"400-with-code", func(c *gin.Context) { ginapi.BadRequestWithCode(c, ginapi.ErrorCodeMissingParam, "amount is required") }},
		{"400-with-param", func(c *gin.Context) { ginapi.BadRequestParam(c, "amount", "amount is required") }},
		{"401-unauthorized", ginapi.Unauthorized},
		{"403-forbidden", ginapi.Forbidden},
		{"404-not-found", func(c *gin.Context) { ginapi.NotFound(c, "gallery") }},
		{"409-conflict", func(c *gin.Context) { ginapi.Conflict(c, "already exists") }},
		{"415-unsupported-media", func(c *gin.Context) { ginapi.UnsupportedMediaType(c, "unsupported media type") }},
		{"422-unprocessable", func(c *gin.Context) { ginapi.UnprocessableEntity(c, "content rejected by moderation") }},
		{"429-rate-limit", func(c *gin.Context) { ginapi.TooManyRequests(c, "slow down") }},
		{"500-internal", func(c *gin.Context) { ginapi.InternalError(c, "pq: constraint \"galleries_slug_key\" violated") }},
		{"501-not-implemented", func(c *gin.Context) { ginapi.NotImplemented(c, "video transcoding is not configured") }},
		{"502-bad-gateway", func(c *gin.Context) { ginapi.BadGateway(c, "upstream failed") }},
		{"503-unavailable", func(c *gin.Context) { ginapi.ServiceUnavailable(c, "try later") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			golden(t, "ginapi/"+tc.name, capture(t, ginServer(tc.fn), nil))
		})
	}
}

// TestGinAPIVersionsAgree proves the two deployed ginapi pins emit the same
// bytes: doujins holds v0.4.0, hentai0 holds v0.5.0, and the diff between the
// tags touches only README.md and middleware/language.go. Hashing the wire
// sources in the module cache keeps that claim from rotting.
func TestGinAPIVersionsAgree(t *testing.T) {
	const older, newer = "v0.4.0", "v0.5.0"
	files := []string{"response/errors.go", "response/response.go", "response/list.go"}
	checked := 0
	for _, f := range files {
		a, errA := readModuleFile(t, "github.com/doujins-org/ginapi@"+older, f)
		b, errB := readModuleFile(t, "github.com/doujins-org/ginapi@"+newer, f)
		if errA != nil || errB != nil {
			t.Fatalf("could not read %s from both module versions: %v / %v", f, errA, errB)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs between ginapi %s and %s: the two deployed pins are NOT wire-identical", f, older, newer)
		}
		checked++
	}
	if checked != len(files) {
		t.Fatalf("checked %d of %d wire sources; a skipped file is a silent pass", checked, len(files))
	}
	t.Logf("ginapi %s and %s agree on %d/%d wire sources", older, newer, checked, len(files))
}
