package response

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newTestCtx 起一个带 request_id 的测试 gin 上下文。
func newTestCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/t", nil)
	c.Set("request_id", "req-1")
	return c, w
}

// TestErrorEnvelopeCarriesDetail R-99 信封增量：错误响应必有 error.id；
// 20200/20201 经保守映射自动携带稳定码；其余整数码只有 id。
func TestErrorEnvelopeCarriesDetail(t *testing.T) {
	c, w := newTestCtx(t)
	Error(c, CodeUnauthorized, "bad token")
	assertContains(t, w.Body.String(), `"code":20200`, `"id":"`, StableUnauthenticated, HintUnauthenticated)

	c, w = newTestCtx(t)
	Error(c, CodeNotFound, "nope")
	assertContains(t, w.Body.String(), `"code":20300`, `"id":"`)
	assertNotContainsStr(t, w.Body.String(), `"hint_key"`)
}

// TestForbiddenEnvelope 20201/FORBIDDEN：登录了但没权限，与 20200 分档。
func TestForbiddenEnvelope(t *testing.T) {
	c, w := newTestCtx(t)
	Forbidden(c, "denied")
	assertContains(t, w.Body.String(), `"code":20201`, StableForbidden, HintForbidden)
}

// TestInternalErrorErrStripsOriginalText 内部原文只进日志不进响应体；
// 错误链上有 WrapCode 标注时信封携带稳定码（R-99 验收 2/3）。
func TestInternalErrorErrStripsOriginalText(t *testing.T) {
	secret := "dial tcp 10.0.0.12:5432: connect refused postgres://u:p@h/db"
	c, w := newTestCtx(t)
	InternalErrorErr(c, errors.New(secret))
	body := w.Body.String()
	assertContains(t, body, `"code":50000`, `"msg":"internal server error"`, `"id":"`)
	if strings.Contains(body, "dial tcp") || strings.Contains(body, "postgres://") {
		t.Fatalf("internal text leaked into response: %q", body)
	}

	c, w = newTestCtx(t)
	InternalErrorErr(c, WrapCode(errors.New(secret), StableDatasourceUnreachable, HintDatasourceUnreachable))
	assertContains(t, w.Body.String(), StableDatasourceUnreachable, HintDatasourceUnreachable)
	assertNotContainsStr(t, w.Body.String(), "dial tcp")
}

// TestWrapCodeNilSafe WrapCode(nil) 必须返回 nil，不得包出非 nil 错误。
func TestWrapCodeNilSafe(t *testing.T) {
	if WrapCode(nil, StableInvalidConfig, HintInvalidConfig) != nil {
		t.Fatal("WrapCode(nil) must return nil")
	}
}

// TestSuccessHasNoErrorField 成功响应不带 error 字段（向后兼容红线）。
func TestSuccessHasNoErrorField(t *testing.T) {
	c, w := newTestCtx(t)
	Success(c, map[string]any{"k": "v"})
	if strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("success response must not carry error field: %q", w.Body.String())
	}
}

func assertContains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("response body missing %q: %s", want, body)
		}
	}
}

func assertNotContainsStr(t *testing.T, body, unexpected string) {
	t.Helper()
	if strings.Contains(body, unexpected) {
		t.Errorf("response body unexpectedly contains %q: %s", unexpected, body)
	}
}
