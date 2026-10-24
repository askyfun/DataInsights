package middleware

import (
	"log/slog"
	"os"
	"strconv"
	"time"

	"data-insights/internal/response"

	"github.com/gin-gonic/gin"
)

// AccessLog 返回 access 日志中间件，version 为 R-100 注入的版本串
// （未合入前传 "dev"）。每次请求产出恰好一行 INFO access；业务码非成功时
// 额外产出一行 ERROR request failed——两行同源同 request_id，可配对检索
// （R-96 验收 2/3）。
//
// 字段口径：route 用 gin 的 FullPath()（路由模板，避免高基数；未注册路径
// 如静态文件回退原始路径）；user_id 取自已鉴权 principal，匿名记 anonymous；
// 绝不记录明文 token；日志只写 stdout（slog 默认 handler 由启动时统一装配
// 为 JSONHandler），行内带 host 与 version 支撑多副本聚合。
func AccessLog(version string) gin.HandlerFunc {
	host, _ := os.Hostname()
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Milliseconds()

		route := c.FullPath()
		if route == "" {
			// 未注册路径（webui 静态托管 / JSON 404）没有路由模板；
			// 记原始路径，否则静态资源故障完全不可查。
			route = c.Request.URL.Path
		}
		userID := "anonymous"
		if p, ok := PrincipalFrom(c); ok {
			userID = strconv.Itoa(p.UserID)
		}
		bizCode := response.BizCodeFrom(c)
		attrs := []any{
			"request_id", c.GetString(response.RequestIDKey),
			"method", c.Request.Method,
			"route", route,
			"http_status", c.Writer.Status(),
			"biz_code", bizCode,
			"user_id", userID,
			"duration_ms", duration,
			"client_ip", c.ClientIP(),
			"host", host,
			"version", version,
		}
		if bizCode != 0 && bizCode != response.CodeSuccess {
			slog.Error("request failed", attrs...)
		}
		slog.Info("access", attrs...)
	}
}
