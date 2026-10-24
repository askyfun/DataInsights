package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

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
	registerHealthRoutes(router, &healthController{startedAt: time.Now()})

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

// --- R-100（issue #225）：版本可见与三级健康检查 ---

func getHealthRoute(t *testing.T, h *healthController, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerHealthRoutes(router, h)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func okHealthController() *healthController {
	return &healthController{
		startedAt: time.Now().Add(-time.Minute),
		dbPing:    func(context.Context) error { return nil },
		migrationStatus: func(context.Context) (int64, bool, error) {
			return 12, true, nil
		},
		extractReady: func() bool { return true },
	}
}

func TestVersionReportKeepsFourFieldsWithoutDatabase(t *testing.T) {
	out := versionReport("")
	for _, want := range []string{"version: ", "commit: ", "build_time: ", "migration_version: unknown"} {
		if !strings.Contains(out, want) {
			t.Fatalf("version report missing %q:\n%s", want, out)
		}
	}
	if lines := strings.Count(out, "\n"); lines != 4 {
		t.Fatalf("expected exactly 4 lines, got %d:\n%s", lines, out)
	}
}

func TestVersionRouteExposesBuildMetadata(t *testing.T) {
	recorder := getHealthRoute(t, okHealthController(), "/version")
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Version   string `json:"version"`
			Commit    string `json:"commit"`
			BuildTime string `json:"build_time"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Code != response.CodeSuccess {
		t.Fatalf("expected success code, got %d", payload.Code)
	}
	if payload.Data.Version != version || payload.Data.Commit != commit || payload.Data.BuildTime != buildTime {
		t.Fatalf("unexpected build metadata: %+v", payload.Data)
	}
}

func TestHealthReadyReturns200WhenAllChecksPass(t *testing.T) {
	recorder := getHealthRoute(t, okHealthController(), "/health/ready")
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected ready payload, got %s", recorder.Body.String())
	}
}

func TestHealthReadyReturns503WhenDatabaseUnreachable(t *testing.T) {
	h := okHealthController()
	h.dbPing = func(context.Context) error {
		return errors.New("dial tcp 10.0.0.12:5432: connection refused")
	}

	// liveness 不受影响：库挂了 /health 仍然 200 且不碰库。
	if live := getHealthRoute(t, h, "/health"); live.Code != http.StatusOK {
		t.Fatalf("expected liveness 200 with db down, got %d", live.Code)
	}

	recorder := getHealthRoute(t, h, "/health/ready")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "database unreachable") {
		t.Fatalf("expected database reason, got %s", recorder.Body.String())
	}
}

func TestHealthReadyReturns503WhenMigrationsPending(t *testing.T) {
	h := okHealthController()
	h.migrationStatus = func(context.Context) (int64, bool, error) { return 11, false, nil }

	recorder := getHealthRoute(t, h, "/health/ready")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "migrations pending") {
		t.Fatalf("expected migrations reason, got %s", recorder.Body.String())
	}
}

func TestHealthReadyReturns503WhenUploadStorageUnavailable(t *testing.T) {
	h := okHealthController()
	h.extractReady = func() bool { return false }

	recorder := getHealthRoute(t, h, "/health/ready")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "upload storage unavailable") {
		t.Fatalf("expected upload storage reason, got %s", recorder.Body.String())
	}
}

func TestHealthReadyDoesNotEchoConnectionDetails(t *testing.T) {
	h := okHealthController()
	h.dbPing = func(context.Context) error {
		return errors.New("failed to connect: postgres://user:pw@10.0.0.12:5432/insights")
	}

	recorder := getHealthRoute(t, h, "/health/ready")
	if strings.Contains(recorder.Body.String(), "postgres://") {
		t.Fatalf("readiness body leaks the connection string: %s", recorder.Body.String())
	}
}

func TestHealthDetailReturns404WhenDisabled(t *testing.T) {
	h := okHealthController()
	h.detailEnabled = false

	recorder := getHealthRoute(t, h, "/health/detail")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when HEALTH_DETAIL is off, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestHealthDetailExposesRuntimeStateWithoutSecrets(t *testing.T) {
	h := okHealthController()
	h.detailEnabled = true
	h.sentryEnabled = true
	h.driverCount = 4
	// 底层错误里带连接串；详情端点只报「连不上」这一事实，绝不复述原文。
	h.dbPing = func(context.Context) error {
		return errors.New("postgres://user:pw@10.0.0.12:5432/insights: connection refused")
	}

	recorder := getHealthRoute(t, h, "/health/detail")
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "postgres://") || strings.Contains(recorder.Body.String(), "insights:") {
		t.Fatalf("detail body leaks connection details: %s", recorder.Body.String())
	}

	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{
		"version", "commit", "build_time", "started_at", "uptime_seconds",
		"database_connected", "database_ping_ms", "migration_version",
		"migrations_up_to_date", "drivers", "sentry_enabled", "auth_enforce",
	} {
		if _, ok := payload.Data[key]; !ok {
			t.Fatalf("detail payload missing %q: %v", key, payload.Data)
		}
	}
	if payload.Data["database_connected"] != false {
		t.Fatalf("expected database_connected=false, got %v", payload.Data["database_connected"])
	}
}
