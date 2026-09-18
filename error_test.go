package apikit_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-rails/apikit"
)

// serve runs h on a real listener and returns the status and raw body a real
// client sees — the wire is the contract, so the tests read it off the socket.
func serve(t *testing.T, h http.HandlerFunc) (int, string, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(body)
}

func TestWriteErrorWire(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{
			name:   "invalid request keeps message and param",
			err:    apikit.E(http.StatusBadRequest, apikit.CodeMissingParam, "slug is required").WithParam("slug"),
			status: 400,
			body:   `{"object":"error","error":{"type":"invalid_request_error","code":"missing_param","message":"slug is required","param":"slug"}}`,
		},
		{
			name:   "not found is its own category",
			err:    apikit.E(http.StatusNotFound, apikit.CodeResourceNotFound, "gallery not found"),
			status: 404,
			body:   `{"object":"error","error":{"type":"not_found_error","code":"resource_not_found","message":"gallery not found"}}`,
		},
		{
			name:   "conflict is its own category",
			err:    apikit.E(http.StatusConflict, apikit.CodeResourceConflict, "slug already taken"),
			status: 409,
			body:   `{"object":"error","error":{"type":"conflict_error","code":"resource_conflict","message":"slug already taken"}}`,
		},
		{
			name:   "rejected is explicit, never inferred from 422",
			err:    apikit.E(http.StatusUnprocessableEntity, apikit.CodeContentRejected, "sexualised minor").WithType(apikit.TypeRejected),
			status: 422,
			body:   `{"object":"error","error":{"type":"rejected_error","code":"content_rejected","message":"sexualised minor"}}`,
		},
		{
			name:   "422 without an explicit type stays invalid_request",
			err:    apikit.E(http.StatusUnprocessableEntity, apikit.CodeInvalidParam, "cannot publish an empty chapter"),
			status: 422,
			body:   `{"object":"error","error":{"type":"invalid_request_error","code":"invalid_param","message":"cannot publish an empty chapter"}}`,
		},
		{
			name:   "unconfigured capability is 501, not a crash",
			err:    apikit.E(http.StatusNotImplemented, apikit.CodeNotConfigured, "no MediaStore configured"),
			status: 501,
			body:   `{"object":"error","error":{"type":"not_implemented_error","code":"not_configured","message":"no MediaStore configured"}}`,
		},
		{
			name:   "503 keeps its operational message",
			err:    apikit.E(http.StatusServiceUnavailable, apikit.CodeServiceUnavailable, "search is unavailable"),
			status: 503,
			body:   `{"object":"error","error":{"type":"api_error","code":"service_unavailable","message":"search is unavailable"}}`,
		},
		{
			name:   "500 is scrubbed to a bare internal error",
			err:    apikit.E(http.StatusInternalServerError, apikit.CodeInternalError, `pq: relation "taxonomy_nodes" does not exist`),
			status: 500,
			body:   `{"object":"error","error":{"type":"api_error","code":"internal_error","message":"internal error"}}`,
		},
		{
			name:   "a plain error never reaches the wire",
			err:    errors.New(`dial tcp 10.0.0.4:5432: connect: connection refused`),
			status: 500,
			body:   `{"object":"error","error":{"type":"api_error","code":"internal_error","message":"internal error"}}`,
		},
		{
			name:   "domain types survive: openrails card_error is not drift",
			err:    apikit.E(http.StatusPaymentRequired, "insufficient_funds", "card declined").WithType("card_error"),
			status: 402,
			body:   `{"object":"error","error":{"type":"card_error","code":"insufficient_funds","message":"card declined"}}`,
		},
		{
			name: "metadata and request id ride along",
			err: apikit.E(http.StatusConflict, apikit.CodeResourceConflict, "conflict").
				WithMetadata(map[string]any{"constraint": "taxonomy_nodes_slug_key"}).
				WithRequestID("req_123"),
			status: 409,
			body:   `{"object":"error","error":{"type":"conflict_error","code":"resource_conflict","message":"conflict","request_id":"req_123","metadata":{"constraint":"taxonomy_nodes_slug_key"}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, ctype, body := serve(t, func(w http.ResponseWriter, r *http.Request) {
				apikit.WriteError(w, tc.err)
			})
			if status != tc.status {
				t.Errorf("status = %d, want %d", status, tc.status)
			}
			if ctype != "application/json" {
				t.Errorf("content-type = %q, want application/json", ctype)
			}
			if got := body; got != tc.body+"\n" {
				t.Errorf("wire body mismatch\n got: %s\nwant: %s", got, tc.body)
			}
		})
	}
}

// The absent code is load-bearing: doujins' SPA prefers error.code over
// error.message when both are present, so a writer that invents a code would
// silently replace every human message with a machine string.
func TestCodeIsOmittedNotInvented(t *testing.T) {
	_, _, body := serve(t, func(w http.ResponseWriter, r *http.Request) {
		apikit.WriteError(w, apikit.E(http.StatusBadRequest, "", "limit must be a positive integer"))
	})
	want := `{"object":"error","error":{"type":"invalid_request_error","message":"limit must be a positive integer"}}` + "\n"
	if body != want {
		t.Errorf("got %s want %s", body, want)
	}
}

func TestTypeForStatus(t *testing.T) {
	cases := map[int]apikit.Type{
		400: apikit.TypeInvalidRequest,
		401: apikit.TypeAuthentication,
		403: apikit.TypeAuthorization,
		404: apikit.TypeNotFound,
		409: apikit.TypeConflict,
		415: apikit.TypeInvalidRequest,
		422: apikit.TypeInvalidRequest, // ambiguous: rejected must be explicit
		429: apikit.TypeRateLimit,
		500: apikit.TypeAPI,
		501: apikit.TypeNotImplemented,
		502: apikit.TypeAPI,
		503: apikit.TypeAPI,
	}
	for status, want := range cases {
		if got := apikit.TypeForStatus(status); got != want {
			t.Errorf("TypeForStatus(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestCodeForStatus(t *testing.T) {
	cases := map[int]apikit.Code{
		400: apikit.CodeInvalidParam,
		401: apikit.CodeAuthenticationRequired,
		403: apikit.CodeResourceAccessDenied,
		404: apikit.CodeResourceNotFound,
		409: apikit.CodeResourceConflict,
		429: apikit.CodeRateLimitExceeded,
		500: apikit.CodeInternalError,
		501: apikit.CodeNotImplemented,
		503: apikit.CodeServiceUnavailable,
	}
	for status, want := range cases {
		if got := apikit.CodeForStatus(status); got != want {
			t.Errorf("CodeForStatus(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestErrorIsMatchesOnCode(t *testing.T) {
	sentinel := apikit.E(http.StatusNotFound, apikit.CodeResourceNotFound, "not found")
	wrapped := apikit.E(http.StatusNotFound, apikit.CodeResourceNotFound, "gallery not found").
		WithCause(errors.New("no rows"))
	if !errors.Is(wrapped, sentinel) {
		t.Error("a fresh error with the same code must match the sentinel")
	}
	other := apikit.E(http.StatusNotFound, apikit.CodeInvalidParam, "nope")
	if errors.Is(other, sentinel) {
		t.Error("a different code must not match")
	}
	if got := apikit.AsError(wrapped); got == nil || got.Code != apikit.CodeResourceNotFound {
		t.Errorf("AsError = %+v", got)
	}
}

// The object discriminator is what hentai0's api-service branches on to tell an
// error body from a success body; it must be present on every error.
func TestEnvelopeAlwaysCarriesObjectDiscriminator(t *testing.T) {
	for _, env := range []apikit.Envelope{
		apikit.NewEnvelope(apikit.ErrorObject{Message: "x"}),
		func() apikit.Envelope { _, e := apikit.EnvelopeFor(errors.New("boom")); return e }(),
		apikit.E(400, "", "x").Envelope(),
	} {
		b, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(b, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["object"] != "error" {
			t.Errorf("object = %v, want error (body %s)", decoded["object"], b)
		}
	}
}
