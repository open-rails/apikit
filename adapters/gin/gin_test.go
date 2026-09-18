package apikitgin_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/open-rails/apikit"
	apikitgin "github.com/open-rails/apikit/adapters/gin"
)

// call runs h on a real gin engine behind a real listener and returns what a
// client reads off the socket.
func call(t *testing.T, target string, h gin.HandlerFunc) (int, string) {
	t.Helper()
	r := gin.New()
	r.GET("/x", h)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + target)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return resp.StatusCode, string(body)
}

func TestErrorWriters(t *testing.T) {
	cases := []struct {
		name   string
		h      gin.HandlerFunc
		status int
		body   string
	}{
		{"BadRequest", func(c *gin.Context) { apikitgin.BadRequest(c, "bad") }, 400,
			`{"object":"error","error":{"type":"invalid_request_error","message":"bad"}}`},
		{"BadRequestWithCode", func(c *gin.Context) { apikitgin.BadRequestWithCode(c, apikit.CodeInvalidFormat, "bad") }, 400,
			`{"object":"error","error":{"type":"invalid_request_error","code":"invalid_format","message":"bad"}}`},
		{"BadRequestParam", func(c *gin.Context) { apikitgin.BadRequestParam(c, "limit", "must be positive") }, 400,
			`{"object":"error","error":{"type":"invalid_request_error","message":"must be positive","param":"limit"}}`},
		{"Unauthorized", apikitgin.Unauthorized, 401,
			`{"object":"error","error":{"type":"authentication_error","message":"unauthorized"}}`},
		{"UnauthorizedWithMessage", func(c *gin.Context) { apikitgin.UnauthorizedWithMessage(c, "token expired") }, 401,
			`{"object":"error","error":{"type":"authentication_error","message":"token expired"}}`},
		{"Forbidden", apikitgin.Forbidden, 403,
			`{"object":"error","error":{"type":"authorization_error","message":"forbidden"}}`},
		{"ForbiddenWithMessage", func(c *gin.Context) { apikitgin.ForbiddenWithMessage(c, "not yours") }, 403,
			`{"object":"error","error":{"type":"authorization_error","message":"not yours"}}`},
		{"NotFound", func(c *gin.Context) { apikitgin.NotFound(c, "gallery") }, 404,
			`{"object":"error","error":{"type":"not_found_error","message":"gallery not found"}}`},
		{"NotFoundWithMessage", func(c *gin.Context) { apikitgin.NotFoundWithMessage(c, "gone") }, 404,
			`{"object":"error","error":{"type":"not_found_error","message":"gone"}}`},
		{"Conflict", func(c *gin.Context) { apikitgin.Conflict(c, "taken") }, 409,
			`{"object":"error","error":{"type":"conflict_error","message":"taken"}}`},
		{"UnsupportedMediaType", func(c *gin.Context) { apikitgin.UnsupportedMediaType(c, "send json") }, 415,
			`{"object":"error","error":{"type":"invalid_request_error","message":"send json"}}`},
		{"UnprocessableEntity", func(c *gin.Context) { apikitgin.UnprocessableEntity(c, "empty chapter") }, 422,
			`{"object":"error","error":{"type":"invalid_request_error","message":"empty chapter"}}`},
		{"Rejected", func(c *gin.Context) { apikitgin.Rejected(c, "policy: minors") }, 422,
			`{"object":"error","error":{"type":"rejected_error","code":"content_rejected","message":"policy: minors"}}`},
		{"TooManyRequests", func(c *gin.Context) { apikitgin.TooManyRequests(c, "slow down") }, 429,
			`{"object":"error","error":{"type":"rate_limit_error","message":"slow down"}}`},
		{"InternalError", func(c *gin.Context) { apikitgin.InternalError(c, "boom") }, 500,
			`{"object":"error","error":{"type":"api_error","message":"boom"}}`},
		{"NotImplemented", func(c *gin.Context) { apikitgin.NotImplemented(c, "not built") }, 501,
			`{"object":"error","error":{"type":"not_implemented_error","message":"not built"}}`},
		{"NotConfigured", func(c *gin.Context) { apikitgin.NotConfigured(c, "no MediaStore configured") }, 501,
			`{"object":"error","error":{"type":"not_implemented_error","code":"not_configured","message":"no MediaStore configured"}}`},
		{"BadGateway", func(c *gin.Context) { apikitgin.BadGateway(c, "upstream down") }, 502,
			`{"object":"error","error":{"type":"api_error","message":"upstream down"}}`},
		{"ServiceUnavailable", func(c *gin.Context) { apikitgin.ServiceUnavailable(c, "maintenance") }, 503,
			`{"object":"error","error":{"type":"api_error","message":"maintenance"}}`},
		{"Fail renders an apikit.Error", func(c *gin.Context) {
			apikitgin.Fail(c, apikit.E(http.StatusNotFound, apikit.CodeResourceNotFound, "no such artist"))
		}, 404,
			`{"object":"error","error":{"type":"not_found_error","code":"resource_not_found","message":"no such artist"}}`},
		{"Fail scrubs an unexpected error", func(c *gin.Context) {
			apikitgin.Fail(c, io.ErrUnexpectedEOF)
		}, 500,
			`{"object":"error","error":{"type":"api_error","code":"internal_error","message":"internal error"}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := call(t, "/x", tc.h)
			if status != tc.status {
				t.Errorf("status = %d, want %d", status, tc.status)
			}
			if body != tc.body {
				t.Errorf("wire body mismatch\n got: %s\nwant: %s", body, tc.body)
			}
		})
	}
}

func TestSuccessWriters(t *testing.T) {
	type artist struct {
		Object string `json:"object"`
		ID     string `json:"id"`
	}
	cases := []struct {
		name   string
		h      gin.HandlerFunc
		status int
		body   string
	}{
		{"Object", func(c *gin.Context) { apikitgin.Object(c, artist{"artist", "a_1"}) }, 200, `{"object":"artist","id":"a_1"}`},
		{"Created", func(c *gin.Context) { apikitgin.Created(c, artist{"artist", "a_1"}) }, 201, `{"object":"artist","id":"a_1"}`},
		{"NoContent", apikitgin.NoContent, 204, ``},
		{"Deleted", func(c *gin.Context) { apikitgin.Deleted(c, "artist", "a_1") }, 200, `{"object":"artist","id":"a_1","deleted":true}`},
		{"Success", func(c *gin.Context) { apikitgin.Success(c, "saved") }, 200, `{"object":"message","message":"saved"}`},
		{"ListResponse", func(c *gin.Context) { apikitgin.ListResponse(c, []string{"a"}, 3, 1, 0) }, 200,
			`{"object":"list","data":["a"],"total":3,"limit":1,"offset":0,"has_more":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := call(t, "/x", tc.h)
			if status != tc.status {
				t.Errorf("status = %d, want %d", status, tc.status)
			}
			if body != tc.body {
				t.Errorf("got %s want %s", body, tc.body)
			}
		})
	}
}

func TestBind(t *testing.T) {
	var got apikit.ListParams
	call(t, "/x?limit=5&offset=10&sort=name", func(c *gin.Context) { got = apikitgin.Bind(c); c.Status(200) })
	if got != (apikit.ListParams{Limit: 5, Offset: 10, Sort: "name"}) {
		t.Errorf("got %+v", got)
	}
}

func TestBindFallsBackToSortBy(t *testing.T) {
	var got apikit.ListParams
	call(t, "/x?sort_by=created_at", func(c *gin.Context) { got = apikitgin.Bind(c); c.Status(200) })
	if got.Sort != "created_at" {
		t.Errorf("got %q, want created_at", got.Sort)
	}
}

func TestBindDefaultNormalizes(t *testing.T) {
	var got apikit.ListParams
	call(t, "/x?limit=9999&offset=-4", func(c *gin.Context) { got = apikitgin.BindDefault(c); c.Status(200) })
	if got.Limit != apikit.MaxLimit || got.Offset != 0 {
		t.Errorf("got %+v, want limit=%d offset=0", got, apikit.MaxLimit)
	}

	call(t, "/x", func(c *gin.Context) { got = apikitgin.BindDefault(c); c.Status(200) })
	if got.Limit != apikit.DefaultLimit {
		t.Errorf("got limit=%d, want %d", got.Limit, apikit.DefaultLimit)
	}
}

func TestBindWithDefaults(t *testing.T) {
	var got apikit.ListParams
	call(t, "/x?limit=500", func(c *gin.Context) { got = apikitgin.BindWithDefaults(c, 10, 50); c.Status(200) })
	if got.Limit != 50 {
		t.Errorf("got limit=%d, want the caller's cap of 50", got.Limit)
	}
}
