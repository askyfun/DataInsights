package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"data-insights/internal/response"
	"data-insights/internal/router"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// TestRedactBeforeSend 喂一段含全部敏感形态的事件（R-98 验收 3），断言出站
// payload 不含任何原文、且保留可检索前缀。
func TestRedactBeforeSend(t *testing.T) {
	secrets := []string{
		"postgres://user:pw@host/db",
		"password=topsecret",
		"Bearer eyJhbGciOi",
		"di_pat_9f8e7d6c5b",
		"sk-Ab3xY91234",
	}
	event := &sentry.Event{
		Message: strings.Join(secrets, " | "),
		Exception: []sentry.Exception{
			{Type: "E", Value: strings.Join(secrets, " | ")},
		},
		Breadcrumbs: []*sentry.Breadcrumb{
			{Message: strings.Join(secrets, " | "), Data: map[string]any{"sql": "password=topsecret"}},
		},
		Contexts: map[string]sentry.Context{
			"custom": {"conn": "postgres://user:pw@host/db"},
		},
	}

	got := RedactBeforeSend(event, nil)
	if got != event {
		t.Fatalf("BeforeSend must return the same event pointer")
	}
	// 原文一个都不许残留
	for _, region := range []string{got.Message, got.Exception[0].Value, got.Breadcrumbs[0].Message} {
		for _, s := range secrets {
			if strings.Contains(region, s) {
				t.Errorf("secret %q survived in %q", s, region)
			}
		}
	}
	// 前缀保留：类型仍可 grep
	if !strings.Contains(got.Message, "postgres://user:<redacted>@") {
		t.Errorf("expected prefix-preserving redaction, got %q", got.Message)
	}
	if !strings.Contains(got.Message, "di_pat_<redacted>") || !strings.Contains(got.Message, "sk-<redacted>") {
		t.Errorf("expected PAT/sk- prefix kept, got %q", got.Message)
	}
}

// TestRedactBeforeSendNilEvent 钉死 nil 事件直接丢弃（返回 nil，不发空事件）。
func TestRedactBeforeSendNilEvent(t *testing.T) {
	if got := RedactBeforeSend(nil, nil); got != nil {
		t.Fatalf("expected nil for nil event, got %+v", got)
	}
}

// TestNewErrorID 验证 R-97 格式 DI-<biz>-<8hex> 与唯一性。
func TestNewErrorID(t *testing.T) {
	pattern := regexp.MustCompile(`^DI-50000-[0-9A-F]{8}$`)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewErrorID(response.CodeInternalError)
		if !pattern.MatchString(id) {
			t.Fatalf("id %q does not match format", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

// TestRecoveryReturnsEnvelope 验证 panic 兜底返回标准信封（R-98 验收 5）：
// HTTP 200 口径不变、code 50000、error_id 在信封里、trace 带上请求 id。
func TestRecoveryReturnsEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery()) // 最外层，模拟 main.go 的装配顺序
	r.Use(requestIDStub("req-abc"))
	r.GET("/boom", func(c *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 envelope, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Trace   string `json:"trace"`
		Data    any    `json:"data"`
		ErrorID string `json:"error_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if payload.Code != response.CodeInternalError {
		t.Fatalf("expected code %d, got %d", response.CodeInternalError, payload.Code)
	}
	if !regexp.MustCompile(`^DI-50000-[0-9A-F]{8}$`).MatchString(payload.ErrorID) {
		t.Fatalf("expected error_id in envelope, got %q", payload.ErrorID)
	}
	if payload.Trace != "req-abc" {
		t.Fatalf("expected trace=req-abc, got %q", payload.Trace)
	}
	// panic 原文不许回显给客户端
	if strings.Contains(w.Body.String(), "boom") {
		t.Fatalf("panic value must not leak into the envelope: %s", w.Body.String())
	}
}

// TestRecoveryNoPanicPassThrough 验证非 panic 请求不被兜底拦改。
func TestRecoveryNoPanicPassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery())
	r.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 1}) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "error_id") {
		t.Fatalf("non-panic request must pass through untouched: %d %s", w.Code, w.Body.String())
	}
}

// requestIDStub 是 requestIDMiddleware 的最小替身：只设置 header，避免 cmd 包依赖。
func requestIDStub(id string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Header.Set("X-Request-ID", id)
		c.Next()
	}
}

// capturedEvent 汇聚 BeforeSend 拦到的事件（并发安全）。
type capturedEvent struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (c *capturedEvent) record(e *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return nil // 返回 nil 丢弃事件，阻止真实出站
}

func (c *capturedEvent) all() []*sentry.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*sentry.Event{}, c.events...)
}

// hubStub 在请求上下文里放置一个克隆 hub，复刻 sentrygin 的装配动作。
func hubStub() gin.HandlerFunc {
	return func(c *gin.Context) {
		hub := sentry.CurrentHub().Clone()
		c.Request = c.Request.WithContext(sentry.SetHubOnContext(c.Request.Context(), hub))
		c.Next()
	}
}

// TestErrorReporterCapturesOnce 集成断言（R-98 验收 1/2/4）：统一错误出口对
// 50000/20500 各产生恰好一个带关联 tag 的事件；4xx 业务码（20100）不上报。
func TestErrorReporterCapturesOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)

	capture := &capturedEvent{}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://test@example.com/1", // BeforeSend 丢弃，不会真实出站
		// 与生产装配（initSentry）一致：先经 RedactBeforeSend 脱敏，再记录。
		BeforeSend: func(e *sentry.Event, h *sentry.EventHint) *sentry.Event {
			return capture.record(RedactBeforeSend(e, h), h)
		},
	}); err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}
	t.Cleanup(func() { sentry.Init(sentry.ClientOptions{}) }) // 还原为无 client，避免污染其他用例

	prev := router.ErrorReporterOf()
	router.SetErrorReporter(NewErrorReporter())
	t.Cleanup(func() { router.SetErrorReporter(prev) })

	build := func(handler router.API[struct{}, struct{}]) *gin.Engine {
		r := gin.New()
		r.Use(requestIDStub("req-xyz"))
		r.Use(hubStub())
		r.Use(ScopeTags()) // 与 main.go 一致：hub 就绪后挂 request_id/error_id/route
		g := r.Group("/api")
		router.RegisterPostRoute(g, "/thing", handler)
		return r
	}

	internal := build(func(_ router.Request[struct{}], _ *router.Response[struct{}]) error {
		return router.NewBusinessError(response.CodeInternalError, "db exploded: postgres://u:p@h/db")
	})

	// POST 端点会先 ShouldBindJSON：必须带合法 body 才能穿透绑定、进入 handler 错误出口。
	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/thing", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	w := httptest.NewRecorder()
	internal.ServeHTTP(w, newReq())
	if w.Code != http.StatusOK {
		t.Fatalf("envelope must stay HTTP 200, got %d", w.Code)
	}

	events := capture.all()
	if len(events) != 1 {
		t.Fatalf("expected exactly one event for one 50000 failure, got %d", len(events))
	}
	ev := events[0]
	for tag, want := range map[string]string{
		"biz_code":   "50000",
		"request_id": "req-xyz",
	} {
		if ev.Tags[tag] != want {
			t.Errorf("tag %s = %q, want %q", tag, ev.Tags[tag], want)
		}
	}
	if ev.Tags["error_id"] == "" {
		t.Errorf("expected error_id tag from ScopeTags, got none")
	}
	if ev.Tags["route"] != "/api/thing" {
		t.Errorf("tag route = %q, want /api/thing", ev.Tags["route"])
	}
	// exception 消息经 BeforeSend 脱敏
	for _, ex := range ev.Exception {
		if strings.Contains(ex.Value, "postgres://u:p@h/db") {
			t.Errorf("connection string leaked into exception value: %q", ex.Value)
		}
	}

	// 4xx 业务码不上报（R-98 验收 4）
	clientErr := build(func(_ router.Request[struct{}], _ *router.Response[struct{}]) error {
		return router.NewBusinessError(response.CodeBadRequest, "bad input")
	})
	w2 := httptest.NewRecorder()
	clientErr.ServeHTTP(w2, newReq())
	if n := len(capture.all()); n != 1 {
		t.Fatalf("20100 must not be reported, event count = %d", n)
	}

	// 第三方错误 20500 上报
	third := build(func(_ router.Request[struct{}], _ *router.Response[struct{}]) error {
		return router.NewBusinessError(response.CodeThirdPartyErr, "upstream timeout")
	})
	w3 := httptest.NewRecorder()
	third.ServeHTTP(w3, newReq())
	if n := len(capture.all()); n != 2 {
		t.Fatalf("20500 must be reported, event count = %d", n)
	}
}

// TestErrorReporterNoHubIsNoop 钉死「未配置即无副作用」：请求没经过 sentrygin
// （无 hub）时不 panic、不上报、不改变响应。
func TestErrorReporterNoHubIsNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prev := router.ErrorReporterOf()
	router.SetErrorReporter(NewErrorReporter())
	t.Cleanup(func() { router.SetErrorReporter(prev) })

	r := gin.New()
	g := r.Group("/api")
	router.RegisterPostRoute(g, "/thing", func(_ router.Request[struct{}], _ *router.Response[struct{}]) error {
		return router.NewBusinessError(response.CodeInternalError, "boom")
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/thing", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("envelope must stay HTTP 200, got %d", w.Code)
	}
}
