package middleware

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"tablewave/internal/response"
	"tablewave/internal/router"
	"tablewave/internal/service/auth"
)

// AuthGuard 是 R-92（issue #217）的渐进强制鉴权中间件，取代资源端点原先的
// OptionalBearer（R-95）：在「解析令牌、注入 principal」之上，按 classify 给出的
// 路由分类 + mode（off/shadow/on）决定是否拒绝匿名请求。
//
// 语义：
//   - 携带无效令牌 → 一律 fail-closed 20200（与 OptionalBearer 一致，不静默降级）；
//   - 分类为 public（/health、?q= 分享、auth 公开端点）→ 不做强制；
//   - 分类为 enforce 且未认证：
//     on     → 20200「未认证，请先登录」，中止；
//     shadow → 放行，但记一条 auth_would_deny（发现裸奔的脚本/MCP/探针）；
//     off    → 放行（保持 L0 匿名口径）。
//
// classify 返回 ok=false 表示「未分类」——按 fail-safe 视为 enforce（on 下拒绝），
// 并由 cmd 的路由策略守卫测试保证实际上不会有未分类路由上线。
func AuthGuard(svc auth.Service, mode string, classify func(method, fullPath string) (string, bool)) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearerToken(c.GetHeader("Authorization"))
		var principal *auth.Principal
		if raw != "" {
			p, err := svc.Verify(c.Request.Context(), raw, c.ClientIP(), c.Request.UserAgent())
			if err != nil {
				if be, ok := err.(router.BusinessError); ok {
					response.Error(c, be.Code, be.Message)
				} else {
					response.Unauthorized(c, "authentication failed")
				}
				c.Abort()
				return
			}
			principal = p
			c.Set(principalKey, p)
			c.Set(rawTokenKey, raw)
			c.Set(response.PrincipalUserIDKey, p.UserID)
		}

		full := c.FullPath()
		class, ok := classify(c.Request.Method, full)
		needsAuth := !ok || class == "enforce"
		if needsAuth && principal == nil {
			switch mode {
			case "on":
				response.Unauthorized(c, "未认证：请先登录")
				c.Abort()
				return
			case "shadow":
				slog.Warn("auth_would_deny",
					"request_id", c.GetString(response.RequestIDKey),
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"route", full,
					"route_group", routeGroup(full),
					"client_ip", c.ClientIP(),
					"has_token", raw != "",
				)
			}
		}
		c.Next()
	}
}

// routeGroup 取路由模板的前两段作为粗粒度分组（/api/datasources/:id → /api/datasources），
// 供 shadow 日志按组聚合。仅用于日志字段，不参与鉴权决策。
func routeGroup(full string) string {
	parts := strings.Split(strings.TrimPrefix(full, "/"), "/")
	if len(parts) >= 2 {
		return "/" + parts[0] + "/" + parts[1]
	}
	return "/" + strings.Join(parts, "/")
}
