package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"tablewave/internal/response"
)

// assertPanicEnvelope 断言 panic 兜底信封：HTTP 200 + code 50000 + 合法 error_id（R-97/R-98）。
// （#223 合并后此 helper 在本树下缺失，补齐以保证 cmd 包测试可编译且守住该契约。）
func assertPanicEnvelope(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("panic envelope must be HTTP 200, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Code    int    `json:"code"`
		ErrorID string `json:"error_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal panic envelope: %v (body=%q)", err, w.Body.String())
	}
	if payload.Code != response.CodeInternalError {
		t.Fatalf("panic envelope code want %d, got %d", response.CodeInternalError, payload.Code)
	}
	if !regexp.MustCompile(`^DI-50000-[0-9A-F]{8}$`).MatchString(payload.ErrorID) {
		t.Fatalf("panic envelope must carry error_id DI-50000-<8hex>, got %q", payload.ErrorID)
	}
}
