package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"data-insights/internal/config"
	"data-insights/internal/keystore"
	"data-insights/internal/monitor"
	"data-insights/internal/response"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// TestHealthRouteUsesUnifiedResponse verifies the operational health endpoint also goes through
// the shared response envelope instead of bypassing response normalization with direct c.JSON.
func TestHealthRouteUsesUnifiedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerHealthRoute(router)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal health response: %v", err)
	}
	if payload["code"] != float64(response.CodeSuccess) {
		t.Fatalf("expected code %d, got %v", response.CodeSuccess, payload["code"])
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", payload["data"])
	}
	if data["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", data["status"])
	}
}

// TestSentryPanicPathReturnsEnvelope 验证 panic 场景返回标准信封（R-98 验收 5）：
// sentrygin 以 Repanic: true 捕获后重新抛出，monitor.Recovery 在最外层接住，
// 返回 HTTP 200 + code 50000 + error_id 的统一信封——消灭「200 空 body / 500
// 空 body」两种非信封形态。
func TestSentryPanicPathReturnsEnvelope(t *testing.T) {
	t.Run("sentrygin mounted with repanic", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(monitor.Recovery())
		r.Use(newSentryginMiddleware()) // Repanic: true，panic 穿透到 Recovery
		r.GET("/boom", func(c *gin.Context) { panic("boom") })

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

		assertPanicEnvelope(t, w)
	})

	t.Run("sentry not active, sentrygin not mounted", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(monitor.Recovery())
		r.GET("/boom", func(c *gin.Context) { panic("boom") })

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

		assertPanicEnvelope(t, w)
	})
}

// assertPanicEnvelope 断言响应是带 error_id 的 50000 信封且 HTTP 200。
func assertPanicEnvelope(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 envelope, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		ErrorID string `json:"error_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if payload.Code != response.CodeInternalError {
		t.Fatalf("expected code %d, got %d", response.CodeInternalError, payload.Code)
	}
	if payload.ErrorID == "" {
		t.Fatalf("expected error_id in panic envelope, got none: %s", w.Body.String())
	}
}

// TestInitSentryEmptyDSNIsNoop 钉死「未配置即无副作用」（R-98 验收 8）：
// SENTRY_DSN 留空时 initSentry 返回 false，不初始化 SDK。
func TestInitSentryEmptyDSNIsNoop(t *testing.T) {
	if initSentry("", "") {
		t.Fatalf("empty DSN must return false")
	}
}

// TestInitSentryValidDSNActivates 验证合法 DSN 时初始化成功且携带 environment。
func TestInitSentryValidDSNActivates(t *testing.T) {
	if !initSentry("https://test@example.com/1", "staging") {
		t.Fatalf("valid DSN must return true")
	}
	// 还原为无 client 的全局 hub，避免污染其他用例。
	if err := sentry.Init(sentry.ClientOptions{}); err != nil {
		t.Fatalf("reset sentry: %v", err)
	}
}

func TestRequestIDMiddlewareGeneratesRandomID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(requestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	got := w.Header().Get("X-Request-ID")
	if got == "" || got == "0000000000000000" {
		t.Fatalf("expected random request id, got %q", got)
	}
}

func TestRequestIDMiddlewarePreservesIncomingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(requestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "abc123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "abc123" {
		t.Fatalf("expected preserved id abc123, got %q", got)
	}
}

func TestResolveSecurityKeyEnvTakesPriority(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	envHex := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	c := &config.Config{}
	c.Security.SecurityKey = envHex

	key, err := resolveSecurityKey(context.Background(), c, db)
	if err != nil {
		t.Fatalf("resolveSecurityKey: %v", err)
	}
	if got := hex.EncodeToString(key); got != envHex {
		t.Fatalf("expected env key %q, got %q", envHex, got)
	}
}

func TestResolveSecurityKeyFallbackToStoreWhenEnvEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM bi_setting WHERE key = $1`)).
		WithArgs(keystore.SettingKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"))

	c := &config.Config{} // SecurityKey empty
	key, err := resolveSecurityKey(context.Background(), c, db)
	if err != nil {
		t.Fatalf("resolveSecurityKey: %v", err)
	}
	if got := hex.EncodeToString(key); got != "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899" {
		t.Fatalf("expected store key, got %q", got)
	}
}

func TestResolveSecurityKeyMalformedEnvRejected(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	c := &config.Config{}
	c.Security.SecurityKey = "zzzz"
	if _, err := resolveSecurityKey(context.Background(), c, db); err == nil {
		t.Fatalf("expected error for malformed key, got nil")
	}
}
