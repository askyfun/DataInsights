package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"data-insights/internal/response"
	"data-insights/internal/service/auth"

	"github.com/gin-gonic/gin"
)

// captureLogs 把 slog 默认 handler 换成写到 buf 的 JSONHandler，返回恢复函数。
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func parseLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if ln == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("parse log line %q: %v", ln, err)
		}
		lines = append(lines, m)
	}
	return lines
}

func newLogTestRouter(authed bool) *gin.Engine {
	r := gin.New()
	r.Use(RequestID())
	r.Use(AccessLog("dev"))
	if authed {
		r.Use(func(c *gin.Context) {
			c.Set(principalKey, &auth.Principal{UserID: 42, Username: "alice"})
			c.Next()
		})
	}
	r.GET("/api/boom", func(c *gin.Context) {
		response.Error(c, response.CodeInternalError, "postgres://u:p@h/db dial failed")
	})
	return r
}

// TestAccessLogFailureProducesPairedLines 钉住 R-96 验收 2/3：一次失败请求
// 恰好两行（ERROR request failed + INFO access），同 request_id 配对，字段齐备。
func TestAccessLogFailureProducesPairedLines(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := captureLogs(t)
	r := newLogTestRouter(false)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/boom", nil))

	lines := parseLogLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 log lines, got %d:\n%s", len(lines), buf.String())
	}
	var errLine, accessLine map[string]any
	for _, m := range lines {
		switch {
		case m["msg"] == "request failed" && m["level"] == "ERROR":
			errLine = m
		case m["msg"] == "access" && m["level"] == "INFO":
			accessLine = m
		}
	}
	if errLine == nil || accessLine == nil {
		t.Fatalf("expected one ERROR 'request failed' and one INFO 'access', got:\n%s", buf.String())
	}
	if errLine["request_id"] == "" || errLine["request_id"] != accessLine["request_id"] {
		t.Fatalf("two lines must share a non-empty request_id: %v vs %v", errLine["request_id"], accessLine["request_id"])
	}
	// 响应头 X-Request-ID 与日志同 id。
	if rid := w.Header().Get("X-Request-ID"); rid != errLine["request_id"] {
		t.Fatalf("response header id %q must equal log id %v", rid, errLine["request_id"])
	}
	for _, m := range []map[string]any{errLine, accessLine} {
		if m["route"] != "/api/boom" {
			t.Fatalf("route must be FullPath /api/boom, got %v", m["route"])
		}
		if m["biz_code"] != float64(response.CodeInternalError) {
			t.Fatalf("biz_code must be %d, got %v", response.CodeInternalError, m["biz_code"])
		}
		if m["http_status"] != float64(http.StatusOK) {
			t.Fatalf("http_status must be 200, got %v", m["http_status"])
		}
		if m["user_id"] != "anonymous" {
			t.Fatalf("user_id must be anonymous, got %v", m["user_id"])
		}
		if m["duration_ms"] == nil || m["host"] == "" || m["version"] != "dev" {
			t.Fatalf("duration_ms/host/version must be present, got %v", m)
		}
	}
}

// TestAccessLogSuccessHasNoErrorLine 成功请求只有一行 access，无 ERROR。
func TestAccessLogSuccessHasNoErrorLine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := captureLogs(t)
	r := gin.New()
	r.Use(RequestID())
	r.Use(AccessLog("dev"))
	r.GET("/api/ok", func(c *gin.Context) { response.Success(c, gin.H{"ok": true}) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/ok", nil))

	lines := parseLogLines(t, buf)
	if len(lines) != 1 || lines[0]["msg"] != "access" {
		t.Fatalf("expected single access line, got:\n%s", buf.String())
	}
	if lines[0]["biz_code"] != float64(response.CodeSuccess) {
		t.Fatalf("biz_code must be %d, got %v", response.CodeSuccess, lines[0]["biz_code"])
	}
}

// TestAccessLogAuthenticatedUserID 已鉴权请求的日志行带 user_id，不带 token。
func TestAccessLogAuthenticatedUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := captureLogs(t)
	r := newLogTestRouter(true)

	req := httptest.NewRequest(http.MethodGet, "/api/boom", nil)
	req.Header.Set("Authorization", "Bearer super-secret-token")
	r.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if !strings.Contains(out, `"user_id":"42"`) {
		t.Fatalf("expected user_id 42 in logs, got:\n%s", out)
	}
	if strings.Contains(out, "super-secret-token") {
		t.Fatalf("plaintext token must never appear in logs:\n%s", out)
	}
}

// TestAccessLogConcurrentUniqueIDs R-96 验收 8：并发 100 个失败请求，
// request_id 全部唯一，且每个 id 恰好配对两行。
func TestAccessLogConcurrentUniqueIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := captureLogs(t)
	r := newLogTestRouter(false)

	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/boom", nil))
		}()
	}
	wg.Wait()

	lines := parseLogLines(t, buf)
	if len(lines) != n*2 {
		t.Fatalf("expected %d log lines, got %d", n*2, len(lines))
	}
	counts := map[string]int{}
	for _, m := range lines {
		id, _ := m["request_id"].(string)
		if id == "" {
			t.Fatalf("log line missing request_id: %v", m)
		}
		counts[id]++
	}
	if len(counts) != n {
		t.Fatalf("expected %d unique request ids, got %d", n, len(counts))
	}
	for id, cnt := range counts {
		if cnt != 2 {
			t.Fatalf("request id %s paired %d times, want exactly 2", fmt.Sprintf("%.8s", id), cnt)
		}
	}
}
