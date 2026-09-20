// Package webui 把前端构建产物（SPA）挂到与 API 同一个 HTTP 服务上。
//
// 单镜像单容器因此只需要一个进程、一个端口：/api 等前缀由内部路由处理，
// 其余路径命中磁盘文件就直接返回，未命中则回落 index.html 交给前端路由。
// 这替代了原先「nginx 托管静态文件 + 反代 /api」的两进程方案 —— 静态托管
// 和路径分流本来就是十几行代码的事，多一个进程换不来任何东西。
package webui

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"data-insights/internal/response"

	"github.com/gin-gonic/gin"
)

// ReservedPrefixes 是永不回落给前端的路径前缀。
//
// 命中它们意味着「请求是发给后端的，但后端没有这条路由」，此时必须回 JSON 404
// 而不是 HTML 页面：否则调用方拿到一坨 HTML，会把「路由写错了」误诊成
// 「返回了非法数据」。匹配按路径段边界进行，所以 /apiary 不属于 /api。
//
// /mcp 尚未实现，此处先占位：等 MCP 端点接进来时不必再改这段判断，
// 而在它实现之前也不会把 index.html 回给 MCP 客户端。
var ReservedPrefixes = []string{"/api", "/mcp", "/health"}

// Handler 以某个目录为根托管前端构建产物。
type Handler struct {
	dir   string
	fsys  fs.FS
	index []byte
}

// New 校验目录并预读 index.html。
//
// 目录不存在、或缺少 index.html，都直接报错返回：静态目录配错属于部署错误，
// 宁可在启动时炸掉，也不要跑起来之后每个页面都 404 —— 后者要排查半天。
// index.html 常驻内存还有两个好处：每次回落不必再读盘，且不受运行期改文件影响。
func New(dir string) (*Handler, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve static dir %q: %w", dir, err)
	}

	fsys := os.DirFS(abs)
	index, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Join(abs, "index.html"), err)
	}

	return &Handler{dir: abs, fsys: fsys, index: index}, nil
}

// Mount 把处理器装到 gin 的 NoRoute 上。所有已注册路由都不受影响，
// 只有「没有匹配到任何路由」的请求才会走到这里。
func (h *Handler) Mount(r *gin.Engine) {
	r.NoRoute(h.serve)
}

func (h *Handler) serve(c *gin.Context) {
	if isReserved(c.Request.URL.Path) {
		response.NotFound(c, "no such endpoint: "+c.Request.Method+" "+c.Request.URL.Path)
		return
	}

	if name, ok := h.resolve(c.Request.URL.Path); ok {
		c.Header("Cache-Control", cacheControlFor(name))
		// 用 c.File 而不是手工读盘：Range、If-Modified-Since、HEAD 都交给
		// http.ServeFile 处理，静态文件该有的语义一个不少。
		c.File(filepath.Join(h.dir, name))
		return
	}

	// 未命中任何文件 —— 交给前端路由（/datasets/12、/share/<token> 这类深链
	// 在服务端并不存在对应文件）。index.html 必须每次重新校验：否则发版后
	// 浏览器拿着旧 index.html 去加载已经不存在的旧 chunk。
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", h.index)
}

// resolve 把 URL 路径映射到静态目录内的相对文件名，并确认该文件确实存在。
//
// 安全性由 fs.ValidPath 兜底：它拒绝 ".." 段、空段和绝对路径，因此
// /../../etc/passwd 与 %2e%2e%2f 这类编码变形都无法逃出静态目录。
func (h *Handler) resolve(urlPath string) (string, bool) {
	unescaped, err := url.PathUnescape(urlPath)
	if err != nil {
		return "", false
	}

	name := strings.TrimPrefix(unescaped, "/")
	if name == "" || !fs.ValidPath(name) {
		return "", false
	}

	info, err := fs.Stat(h.fsys, name)
	if err != nil || info.IsDir() {
		return "", false
	}

	return name, true
}

// isReserved 判断路径是否落在后端保留前缀内，按路径段边界匹配。
func isReserved(urlPath string) bool {
	for _, prefix := range ReservedPrefixes {
		if urlPath == prefix || strings.HasPrefix(urlPath, prefix+"/") {
			return true
		}
	}
	return false
}

// cacheControlFor 给出静态文件的缓存策略。
//
// Vite 打到 assets/ 下的文件名带内容哈希，内容一变名字就变，可以放心长缓存；
// 其余（index.html、favicon 之类）一律不缓存，避免发版后新老版本混用。
func cacheControlFor(name string) string {
	if strings.HasPrefix(name, "assets/") {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}
