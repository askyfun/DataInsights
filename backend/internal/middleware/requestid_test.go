package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"data-insights/internal/response"

	"github.com/gin-gonic/gin"
)

// newRequestIDRouter 组一个挂了 RequestID 中间件的最小路由，handler 把
// context 里的 request_id 回显出来，方便断言「同一口径」。
func newRequestIDRouter() (*gin.Engine, *string) {
	var seen string
	r := gin.New()
	r.Use(RequestID())
	r.GET("/ping", func(c *gin.Context) {
		seen = c.GetString(response.RequestIDKey)
		c.Status(http.StatusOK)
	})
	return r, &seen
}

func TestRequestIDGeneratesRandomID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, seen := newRequestIDRouter()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	got := w.Header().Get("X-Request-ID")
	if got == "" || got == "0000000000000000" {
		t.Fatalf("expected random request id, got %q", got)
	}
	if *seen != got {
		t.Fatalf("context id %q must equal response header %q", *seen, got)
	}
}

func TestRequestIDPreservesValidIncomingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, seen := newRequestIDRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "abc123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "abc123" {
		t.Fatalf("expected preserved id abc123, got %q", got)
	}
	if *seen != "abc123" {
		t.Fatalf("expected context id abc123, got %q", *seen)
	}
}

// TestRequestIDRejectsInvalidIncomingID 钉住 R-96 验收 6：入站 id 超长或含
// 白名单外字符时服务端重新生成，不透传也不报错。
func TestRequestIDRejectsInvalidIncomingID(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"超长", string(make([]byte, 129))},
		{"怪字符", "id with spaces/../etc"},
		{"换行注入", "evil\n200 OK"},
		{"空字节", "a\x00b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r, _ := newRequestIDRouter()

			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			req.Header.Set("X-Request-ID", tc.in)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			got := w.Header().Get("X-Request-ID")
			if got == tc.in {
				t.Fatalf("invalid id %q must not be passed through", tc.in)
			}
			if got == "" {
				t.Fatalf("expected regenerated id, got empty")
			}
		})
	}
}
