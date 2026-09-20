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
