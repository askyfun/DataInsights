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
	"data-insights/internal/response"

	"github.com/DATA-DOG/go-sqlmock"
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

// TestSentryPanicPathReturns500 验证 panic 场景不会变成 200 空响应：
// sentrygin 默认吞 panic，生产配置必须 Repanic: true 让 panic 穿透到外层
// gin Recovery（gin.Default()），从而返回 500。
func TestSentryPanicPathReturns500(t *testing.T) {
	t.Run("sentrygin mounted with repanic", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(gin.Recovery()) // 模拟 gin.Default() 的外层 Recovery
		r.Use(newSentryginMiddleware())
		r.GET("/boom", func(c *gin.Context) { panic("boom") })

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 after panic, got %d", w.Code)
		}
	})

	t.Run("sentry not active, sentrygin not mounted", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(gin.Recovery())
		r.GET("/boom", func(c *gin.Context) { panic("boom") })

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 after panic, got %d", w.Code)
		}
	})
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
