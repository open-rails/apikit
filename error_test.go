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
			body:   `{"error":{"type":"invalid_request_error","code":"missing_param","message":"slug is required","param":"slug"}}`,
		},
		{
			name:   "404 stays invalid_request_error, as authkit and openrails deploy it",
			err:    apikit.E(http.StatusNotFound, apikit.CodeResourceNotFound, "gallery not found"),
			status: 404,
			body:   `{"error":{"type":"invalid_request_error","code":"resource_not_found","message":"gallery not found"}}`,
		},
		{
			name:   "409 stays invalid_request_error; the conflict is in the code",
			err:    apikit.E(http.StatusConflict, apikit.CodeResourceConflict, "slug already taken"),
			status: 409,
			body:   `{"error":{"type":"invalid_request_error","code":"resource_conflict","message":"slug already taken"}}`,
		},
		{
			name:   "moderation 422 is the existing transport type plus a stable code",
			err:    apikit.E(http.StatusUnprocessableEntity, apikit.CodeModerationRejected, "the artwork was refused"),
			status: 422,
			body:   `{"error":{"type":"invalid_request_error","code":"moderation_rejected","message":"the artwork was refused"}}`,
		},
		{
			name:   "422 without a moderation code is an ordinary invalid request",
			err:    apikit.E(http.StatusUnprocessableEntity, apikit.CodeInvalidParam, "cannot publish an empty chapter"),
			status: 422,
			body:   `{"error":{"type":"invalid_request_error","code":"invalid_param","message":"cannot publish an empty chapter"}}`,
		},
		{
			name:   "501 is api_error plus not_implemented, not a new transport type",
			err:    apikit.E(http.StatusNotImplemented, apikit.CodeNotImplemented, "video transcoding is not built into this deployment"),
			status: 501,
			body:   `{"error":{"type":"api_error","code":"not_implemented","message":"video transcoding is not built into this deployment"}}`,
		},
		{
			name:   "an unconfigured capability is 501 with its own code",
			err:    apikit.E(http.StatusNotImplemented, apikit.CodeNotConfigured, "no MediaStore configured"),
			status: 501,
			body:   `{"error":{"type":"api_error","code":"not_configured","message":"no MediaStore configured"}}`,
		},
		{
			name:   "503 keeps its operational message",
			err:    apikit.E(http.StatusServiceUnavailable, apikit.CodeServiceUnavailable, "search is unavailable"),
			status: 503,
			body:   `{"error":{"type":"api_error","code":"service_unavailable","message":"search is unavailable"}}`,
		},
		{
			name:   "500 is scrubbed to a bare internal error",
			err:    apikit.E(http.StatusInternalServerError, apikit.CodeInternalError, `pq: relation "taxonomy_nodes" does not exist`),
			status: 500,
			body:   `{"error":{"type":"api_error","code":"internal_error","message":"internal error"}}`,
		},
		{
			name:   "a plain error never reaches the wire",
			err:    errors.New(`dial tcp 10.0.0.4:5432: connect: connection refused`),
			status: 500,
			body:   `{"error":{"type":"api_error","code":"internal_error","message":"internal error"}}`,
		},
		{
			name:   "402 infers openrails' card_error, the one domain-flavoured status",
			err:    apikit.E(http.StatusPaymentRequired, "insufficient_funds", "card declined"),
			status: 402,
			body:   `{"error":{"type":"card_error","code":"insufficient_funds","message":"card declined"}}`,
		},
		{
			name: "request id survives; unsafe metadata does not",
			err: apikit.E(http.StatusConflict, apikit.CodeResourceConflict, "conflict").
				WithMetadata(map[string]any{"constraint": "taxonomy_nodes_slug_key"}).
				WithRequestID("req_123"),
			status: 409,
			body:   `{"error":{"type":"invalid_request_error","code":"resource_conflict","message":"conflict","request_id":"req_123"}}`,
		},
		{
			name: "allowlisted metadata rides along",
			err: apikit.E(http.StatusTooManyRequests, apikit.CodeRateLimitExceeded, "slow down").
				WithMetadata(map[string]any{"retry_after": 30}),
			status: 429,
			body:   `{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"slow down","metadata":{"retry_after":30}}}`,
		},
		{
			name:   "a 500 keeps its request id so the log is still reachable",
			err:    apikit.E(http.StatusInternalServerError, apikit.CodeInternalError, "boom").WithRequestID("req_500"),
			status: 500,
			body:   `{"error":{"type":"api_error","code":"internal_error","message":"internal error","request_id":"req_500"}}`,
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
// silently replace every human message with a machine string
// (compat/golden/parsers.json records exactly that).
func TestCodeIsOmittedNotInvented(t *testing.T) {
	_, _, body := serve(t, func(w http.ResponseWriter, r *http.Request) {
		apikit.WriteError(w, apikit.E(http.StatusBadRequest, "", "limit must be a positive integer"))
	})
	want := `{"error":{"type":"invalid_request_error","message":"limit must be a positive integer"}}` + "\n"
	if body != want {
		t.Errorf("got %s want %s", body, want)
	}
}

// param is a *string in Go and omitempty on the wire: absent and empty are one
// thing to a client, and "param":null is a thing nobody wants.
func TestParamIsPointerAbsentWhenUnset(t *testing.T) {
	var obj apikit.ErrorObject
	if obj.Param != nil {
		t.Fatal("zero ErrorObject must have a nil param")
	}
	e := apikit.E(400, apikit.CodeInvalidParam, "bad").WithParam("slug")
	if got := e.Param(); got != "slug" {
		t.Fatalf("Param() = %q", got)
	}
	if env := e.Envelope(); env.Error.Param == nil || *env.Error.Param != "slug" {
		t.Fatalf("param pointer not set: %+v", env.Error.Param)
	}
	if env := e.WithParam("").Envelope(); env.Error.Param != nil {
		t.Fatal("an empty param must be absent, never null")
	}
	b, _ := json.Marshal(apikit.NewEnvelope(apikit.ErrorObject{Type: apikit.TypeAPI, Message: "x"}))
	if string(b) != `{"error":{"type":"api_error","message":"x"}}` {
		t.Fatalf("absent param leaked: %s", b)
	}
}

func TestTypeForStatus(t *testing.T) {
	cases := map[int]apikit.Type{
		400: apikit.TypeInvalidRequest,
		401: apikit.TypeAuthentication,
		402: apikit.TypeCard,
		403: apikit.TypeAuthorization,
		404: apikit.TypeInvalidRequest,
		409: apikit.TypeInvalidRequest,
		415: apikit.TypeInvalidRequest,
		422: apikit.TypeInvalidRequest,
		429: apikit.TypeRateLimit,
		500: apikit.TypeAPI,
		501: apikit.TypeAPI,
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
		402: apikit.CodePaymentFailed,
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

// The canonical envelope is the deployed authkit/openrails shape. The GinAPI
// discriminator is available, and deliberately never applied by a writer.
func TestCanonicalEnvelopeHasNoDiscriminator(t *testing.T) {
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
		if _, ok := decoded["object"]; ok {
			t.Errorf("canonical envelope must not carry object: %s", b)
		}
		if _, ok := decoded["error"]; !ok {
			t.Errorf("canonical envelope must nest under error: %s", b)
		}
	}
	compat, _ := json.Marshal(apikit.WithObject(apikit.NewEnvelope(apikit.ErrorObject{Type: apikit.TypeAPI, Message: "x"})))
	if string(compat) != `{"object":"error","error":{"type":"api_error","message":"x"}}` {
		t.Errorf("WithObject = %s", compat)
	}
}
