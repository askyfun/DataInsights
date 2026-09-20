package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const (
	indexBody  = "<!doctype html><div id=\"root\"></div>"
	secretBody = "MUST-NOT-LEAK"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// newFixture 搭出这样的目录结构：
//
//	<root>/secret.txt          静态目录之外，任何路径都不许读到
//	<root>/web/index.html      SPA 入口
//	<root>/web/assets/app-1.js 带哈希的构建产物
//	<root>/web/favicon.svg     无哈希的根级静态文件
//
// 返回已挂载静态托管的 gin 引擎。
func newFixture(t *testing.T) *gin.Engine {
	t.Helper()

	root := t.TempDir()
	webDir := filepath.Join(root, "web")

	writeFile(t, filepath.Join(root, "secret.txt"), secretBody)
	writeFile(t, filepath.Join(webDir, "index.html"), indexBody)
	writeFile(t, filepath.Join(webDir, "assets", "app-1.js"), "console.log(1)")
	writeFile(t, filepath.Join(webDir, "favicon.svg"), "<svg/>")

	h, err := New(webDir)
	if err != nil {
		t.Fatalf("New(%q) returned error: %v", webDir, err)
	}

	r := gin.New()
	r.GET("/api/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 20000})
	})
	h.Mount(r)
	return r
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func do(r *gin.Engine, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

// ---------------------------------------------------------------- 目录校验

func TestNew_MissingDir(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for a nonexistent static dir")
	}
}

func TestNew_MissingIndex(t *testing.T) {
	// 目录在但没构建产物 —— 必须启动即失败，否则运行期每个页面都 404。
	dir := t.TempDir()
	if _, err := New(dir); err == nil {
		t.Fatal("expected error when index.html is absent")
	}
}

// ---------------------------------------------------------------- 静态文件

func TestServe_HashedAssetIsImmutable(t *testing.T) {
	w := do(newFixture(t), http.MethodGet, "/assets/app-1.js")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Body.String(); got != "console.log(1)" {
		t.Fatalf("unexpected body: %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected Cache-Control: %q", got)
	}
}

func TestServe_RootFileIsNotCached(t *testing.T) {
	// 没有内容哈希的文件不能长缓存，否则发版后新老版本混用。
	w := do(newFixture(t), http.MethodGet, "/favicon.svg")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("unexpected Cache-Control: %q", got)
	}
}

func TestServe_HeadRequestCarriesNoBody(t *testing.T) {
	w := do(newFixture(t), http.MethodHead, "/assets/app-1.js")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// httptest 的 Recorder 不模拟 net/http 对 HEAD 的正文抑制，
	// 这里只确认静态文件走的是 http.ServeFile 那条路径（有 Content-Length）。
	if w.Header().Get("Content-Length") == "" {
		t.Fatal("expected Content-Length on HEAD response")
	}
}

// ---------------------------------------------------------------- SPA 回落

func TestServe_FallsBackToIndexForUnknownPaths(t *testing.T) {
	r := newFixture(t)

	// 根路径、前端深链、目录请求：服务端都没有对应文件，一律交给前端路由。
	for _, target := range []string{"/", "/datasets/12", "/share/tok", "/assets/"} {
		w := do(r, http.MethodGet, target)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", target, w.Code)
		}
		if got := w.Body.String(); got != indexBody {
			t.Fatalf("%s: expected index.html, got %q", target, got)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("%s: unexpected Cache-Control: %q", target, got)
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Fatalf("%s: unexpected Content-Type: %q", target, got)
		}
	}
}

func TestServe_RegisteredRoutesAreNotIntercepted(t *testing.T) {
	w := do(newFixture(t), http.MethodGet, "/api/ping")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Body.String(); !strings.Contains(got, "20000") {
		t.Fatalf("NoRoute stole a registered route: %q", got)
	}
}

// ---------------------------------------------------------------- 保留前缀

func TestServe_ReservedPrefixesReturnJSONNotFound(t *testing.T) {
	r := newFixture(t)

	// /api/nope 是「后端没有这条路由」，必须回 JSON；/mcp 尚未实现，
	// 同样不能把 index.html 回给 MCP 客户端。/apiary 不属于保留前缀。
	for _, target := range []string{"/api/nope", "/mcp", "/mcp/tools/list", "/health/deep"} {
		w := do(r, http.MethodGet, target)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected HTTP 200 envelope, got %d", target, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("%s: expected JSON, got Content-Type %q", target, ct)
		}
		if strings.Contains(w.Body.String(), "<!doctype") {
			t.Fatalf("%s: SPA fallback leaked into a reserved prefix: %q", target, w.Body.String())
		}

		var body struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: invalid JSON envelope: %v", target, err)
		}
		if body.Code != 20300 {
			t.Fatalf("%s: expected code 20300, got %d (%s)", target, body.Code, body.Msg)
		}
	}
}

func TestIsReserved_MatchesOnSegmentBoundary(t *testing.T) {
	tests := map[string]bool{
		"/api":            true,
		"/api/":           true,
		"/api/charts":     true,
		"/mcp":            true,
		"/health":         true,
		"/apiary":         false,
		"/mcpx":           false,
		"/api-docs":       false,
		"/":               false,
		"/datasets/12":    false,
		"/healthcheck/xd": false,
	}

	for path, want := range tests {
		if got := isReserved(path); got != want {
			t.Errorf("isReserved(%q) = %v, want %v", path, got, want)
		}
	}
}

// ---------------------------------------------------------------- 路径穿越

func TestServe_DoesNotEscapeStaticDir(t *testing.T) {
	r := newFixture(t)

	// 明文与百分号编码两种写法都要挡住。命中静态目录之外的文件即为漏洞，
	// 回落 index.html 或直接 400 都可以接受。
	for _, target := range []string{"/../secret.txt", "/%2e%2e%2fsecret.txt", "/assets/../../secret.txt"} {
		w := do(r, http.MethodGet, target)

		if strings.Contains(w.Body.String(), secretBody) {
			t.Fatalf("%s: static dir escaped, served %q", target, w.Body.String())
		}
		if w.Code == http.StatusOK && w.Body.String() != indexBody {
			t.Fatalf("%s: unexpected 200 body %q", target, w.Body.String())
		}
	}
}
