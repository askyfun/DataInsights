package webui

// fuzz 回归探针（chaos 测试转正，2026-09-20）：钉住静态托管的路径穿越防护。

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// zzFixture 建出 <root>/secret.txt（静态目录之外）+ <root>/web/{index.html,assets/app-1.js}。
func zzFixture(t testing.TB) *Handler {
	t.Helper()
	root := t.TempDir()
	web := filepath.Join(root, "web")
	if err := os.MkdirAll(filepath.Join(web, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, body string) {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "secret.txt"), "MUST-NOT-LEAK")
	write(filepath.Join(web, "index.html"), "<html>index</html>")
	write(filepath.Join(web, "assets", "app-1.js"), "js")
	h, err := New(web)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

// FuzzZZResolve 钉住 resolve 的逃逸面：返回的文件名必须仍是静态目录内的相对路径。
func FuzzZZResolve(f *testing.F) {
	h := zzFixture(f)
	for _, s := range []string{
		"/", "", "/index.html", "/assets/app-1.js",
		"/../secret.txt", "/..%2fsecret.txt", "/%2e%2e/secret.txt",
		"/..%252fsecret.txt", "/a/../../../../etc/passwd",
		"//etc/passwd", "/etc/passwd", "\\..\\secret.txt",
		"/assets/../../../secret.txt", "/%00", "/index.html%00.txt",
		"/....//secret.txt", "/./index.html", "/a/./../index.html",
		"/%2fetc%2fpasswd", "/index.html/../../secret.txt",
		"/assets/", "/assets", "/assets\\..\\..\\secret.txt",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, p string) {
		name, ok := h.resolve(p)
		if !ok {
			return
		}
		if name == "" {
			t.Fatalf("resolve(%q) 声称命中但返回空名", p)
		}
		if !fs.ValidPath(name) {
			t.Fatalf("resolve(%q) 返回非法路径 %q", p, name)
		}
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
			t.Fatalf("resolve(%q) 返回逃逸路径 %q", p, name)
		}
		full := filepath.Join(h.dir, name)
		rel, err := filepath.Rel(h.dir, full)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("resolve(%q) → %q 逃出静态目录（%q）", p, name, full)
		}
		// 上盘读一次，确认不是 secret
		if b, err := os.ReadFile(full); err == nil && strings.Contains(string(b), "MUST-NOT-LEAK") {
			t.Fatalf("resolve(%q) → %q 读到了静态目录外的 secret.txt", p, name)
		}
	})
}

// FuzzZZIsReservedVsServe 直击「守卫说保留、实际却回 HTML」的不一致：
// 未命中任何路由的请求若 isReserved 为真，响应体必须是 JSON 404，不得是 index.html。
func FuzzZZIsReservedVsServe(f *testing.F) {
	h := zzFixture(f)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 20000}) })
	h.Mount(r)

	for _, s := range []string{
		"/api", "/api/", "/api/nope", "/api/nope/deep", "/apiary",
		"/mcp", "/mcp/x", "/health", "/healthz", "/health/",
		"//api/nope", "/api/../api/x", "/API/nope", "/api%2fnope",
		"/api/./x", "/./api/nope", "/api?x=1", "/api#f", "/api%00",
		"/%61pi/nope", "/a/../api/nope", "/api\\nope", "///api/nope",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, p string) {
		parsed, err := url.Parse("http://x" + p)
		if err != nil {
			return // 真实 net/http 也拒绝这类目标
		}
		req := &http.Request{Method: http.MethodGet, URL: parsed, Host: "x", Header: http.Header{}}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		decoded := req.URL.Path // 与 handler 读的是同一个字段
		if !isReserved(decoded) {
			return
		}
		body := w.Body.String()
		if strings.Contains(body, "<html>index</html>") {
			t.Fatalf("路径 %q（handler 视角 %q）isReserved=true 但回落了 index.html（status=%d）",
				p, decoded, w.Code)
		}
		if !strings.Contains(body, "no such endpoint") {
			t.Fatalf("路径 %q（handler 视角 %q）isReserved=true 但响应不是后端 JSON 404：status=%d body=%q",
				p, decoded, w.Code, body)
		}
	})
}
