package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"data-insights/internal/query"

	"github.com/gin-gonic/gin"
)

// decodeRecorderBody unmarshals a recorder body into a generic map so response contract tests can
// assert the final JSON emitted by Gin instead of only checking intermediate Go values.
func decodeRecorderBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response body: %v", err)
	}
	return payload
}

// newTestContext creates a Gin test context backed by an HTTP recorder for response helpers.
func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return ctx, recorder
}

// TestSuccessNormalizesNestedCollectionFields verifies the shared success response path converts
// nil collection fields in nested chart payloads into empty arrays before JSON serialization.
func TestSuccessNormalizesNestedCollectionFields(t *testing.T) {
	ctx, recorder := newTestContext()

	Success(ctx, query.AxisResponse{
		XAxis: nil,
		Series: []query.AxisSeries{{
			Name: "series-a",
			Data: nil,
		}},
	})

	payload := decodeRecorderBody(t, recorder)
	data := payload["data"].(map[string]any)

	if _, ok := data["x_axis"].([]any); !ok {
		t.Fatalf("expected x_axis to be JSON array, got %T", data["x_axis"])
	}
	series, ok := data["series"].([]any)
	if !ok || len(series) != 1 {
		t.Fatalf("expected one series entry, got %T (%v)", data["series"], data["series"])
	}
	seriesItem := series[0].(map[string]any)
	if _, ok := seriesItem["data"].([]any); !ok {
		t.Fatalf("expected nested series data to be JSON array, got %T", seriesItem["data"])
	}
}

// TestSuccessWithPageNormalizesNilItems verifies paginated responses also keep empty collections
// as JSON arrays when handlers pass a typed nil slice into the shared response wrapper.
func TestSuccessWithPageNormalizesNilItems(t *testing.T) {
	ctx, recorder := newTestContext()

	var items []string
	SuccessWithPage(ctx, items, 0, 1, 20)

	payload := decodeRecorderBody(t, recorder)
	data := payload["data"].(map[string]any)
	if _, ok := data["items"].([]any); !ok {
		t.Fatalf("expected items to be JSON array, got %T", data["items"])
	}
	if len(data["items"].([]any)) != 0 {
		t.Fatalf("expected items to be empty array, got %v", data["items"])
	}
}

// TestErrorUsesEmptyObjectData verifies error responses no longer expose top-level null data and
// instead return an empty object that matches the documented response envelope contract.
func TestErrorUsesEmptyObjectData(t *testing.T) {
	ctx, recorder := newTestContext()

	Error(ctx, CodeBadRequest, "invalid request")

	payload := decodeRecorderBody(t, recorder)
	if _, ok := payload["data"].(map[string]any); !ok {
		t.Fatalf("expected error data to be JSON object, got %T", payload["data"])
	}
}

// newTraceRouter 挂上 requestID 中间件后注册一个会写统一信封的路由，
// 模拟真实请求链路，供 trace 取值口径的用例使用。
func newTraceRouter() (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		// 与 middleware.RequestID 同口径的最小复刻：写 context 键 + 响应头。
		// 这里不复用 middleware 包，避免 response → middleware 的依赖倒置。
		if id := c.GetHeader("X-Request-ID"); id != "" {
			c.Set(RequestIDKey, id)
		} else {
			c.Set(RequestIDKey, "generated-test-id")
		}
		c.Header("X-Request-ID", c.GetString(RequestIDKey))
		c.Next()
	})
	recorder := httptest.NewRecorder()
	r.GET("/probe", func(c *gin.Context) { Success(c, gin.H{"ok": true}) })
	return r, recorder
}

// TestTracePrefersContextOverHeader 钉住 R-96 验收 1：trace 的主取值口径是
// context 键，入站头只是未挂中间件时的回落——「中间件把 id 写回入站头」的
// 副作用被移除后，链路依然成立。
func TestTracePrefersContextOverHeader(t *testing.T) {
	r, w := newTraceRouter()

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("X-Request-ID", "probe-123")
	r.ServeHTTP(w, req)

	payload := decodeRecorderBody(t, w)
	if payload["trace"] != "probe-123" {
		t.Fatalf("expected trace probe-123, got %v", payload["trace"])
	}
	if w.Header().Get("X-Request-ID") != "probe-123" {
		t.Fatalf("response header must echo the request id")
	}
}

// TestTraceFallsBackWhenMiddlewareMissing 未挂 requestID 中间件时（部分测试
// 场景），getTraceID 回落读入站头，不为空。
func TestTraceFallsBackWhenMiddlewareMissing(t *testing.T) {
	ctx, recorder := newTestContext()
	ctx.Request.Header.Set("X-Request-ID", "bare-handler-id")

	Success(ctx, gin.H{})

	payload := decodeRecorderBody(t, recorder)
	if payload["trace"] != "bare-handler-id" {
		t.Fatalf("expected header fallback trace, got %v", payload["trace"])
	}
}

// TestErrorRedactsSensitiveMessage 钉住「写响应的错误文本经同一 helper 收口」：
// 含连接串密码的错误信息在响应体里已脱敏。
func TestErrorRedactsSensitiveMessage(t *testing.T) {
	ctx, recorder := newTestContext()

	Error(ctx, CodeInternalError, "connect failed: postgres://app:hunter2@db:5432/x")

	payload := decodeRecorderBody(t, recorder)
	if msg := payload["msg"].(string); msg != "connect failed: postgres://app:<redacted>@db:5432/x" {
		t.Fatalf("expected redacted msg, got %q", msg)
	}
}

// TestBizCodeFromRecordsLastError 供 access 日志读取的业务码应与最后写出的错误一致。
func TestBizCodeFromRecordsLastError(t *testing.T) {
	ctx, _ := newTestContext()
	if got := BizCodeFrom(ctx); got != 0 {
		t.Fatalf("expected 0 before any error, got %d", got)
	}
	Error(ctx, CodeNotFound, "missing")
	if got := BizCodeFrom(ctx); got != CodeNotFound {
		t.Fatalf("expected biz code %d, got %d", CodeNotFound, got)
	}
}
