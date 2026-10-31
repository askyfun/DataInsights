package response

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
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

// errorIDPattern 是 issue #222（R-97）钉死的错误编号形状：DI-<业务码>-<8位大写hex>。
var errorIDPattern = regexp.MustCompile(`^DI-\d{5}-[0-9A-F]{8}$`)

// TestErrorResponseCarriesErrorID verifies every error response carries a well-formed
// error_id, the same id is stashed in the gin context for R-96/R-98 to pick up, and
// consecutive errors get distinct ids (报障编号必须可区分两次失败).
func TestErrorResponseCarriesErrorID(t *testing.T) {
	for _, fn := range []struct {
		name string
		call func(c *gin.Context)
	}{
		{"Error", func(c *gin.Context) { Error(c, CodeBadRequest, "bad") }},
		{"ErrorWithData", func(c *gin.Context) { ErrorWithData(c, CodeInternalError, "boom", nil) }},
		{"BadRequest", func(c *gin.Context) { BadRequest(c, "bad request") }},
		{"InternalError", func(c *gin.Context) { InternalError(c, "internal") }},
	} {
		t.Run(fn.name, func(t *testing.T) {
			ctx, recorder := newTestContext()

			fn.call(ctx)

			payload := decodeRecorderBody(t, recorder)
			id, _ := payload["error_id"].(string)
			if !errorIDPattern.MatchString(id) {
				t.Fatalf("expected error_id matching %s, got %q", errorIDPattern, id)
			}
			if id != ctx.GetString("error_id") {
				t.Fatalf("context error_id %q != body error_id %q", ctx.GetString("error_id"), id)
			}
			// 编号含业务码，可反查：DI-50000-… 对应 50000。
			codeInt := int(payload["code"].(float64))
			if want := fmt.Sprintf("DI-%d-", codeInt); !strings.HasPrefix(id, want) {
				t.Fatalf("error_id %q should start with %q", id, want)
			}
		})
	}

	ctxA, _ := newTestContext()
	ctxB, _ := newTestContext()
	Error(ctxA, CodeInternalError, "first")
	Error(ctxB, CodeInternalError, "second")
	if ctxA.GetString("error_id") == ctxB.GetString("error_id") {
		t.Fatalf("expected distinct error ids, got %q twice", ctxA.GetString("error_id"))
	}
}

// TestSuccessOmitsErrorID verifies the success envelope stays clean: error_id is omitempty
// and must not appear on 20000 responses (前端只在失败分型里读它).
func TestSuccessOmitsErrorID(t *testing.T) {
	ctx, recorder := newTestContext()

	Success(ctx, map[string]string{"ok": "yes"})

	payload := decodeRecorderBody(t, recorder)
	if _, ok := payload["error_id"]; ok {
		t.Fatalf("success envelope must not carry error_id, got %v", payload["error_id"])
	}
}

// TestErrorLogsSameErrorID verifies the ERROR log line emitted at the response boundary
// carries the exact error_id returned in the envelope（issue #222 验收 6：响应与日志同源）.
func TestErrorLogsSameErrorID(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx, recorder := newTestContext()
	Error(ctx, CodeInternalError, "boom")

	payload := decodeRecorderBody(t, recorder)
	id, _ := payload["error_id"].(string)
	if !strings.Contains(buf.String(), "error_id="+id) {
		t.Fatalf("expected log line to contain error_id=%s, log was: %s", id, buf.String())
	}
}
