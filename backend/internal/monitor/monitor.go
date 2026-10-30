// Package monitor 收口 R-98（issue #223）的监控横切面：非 panic 错误的单点
// Sentry 上报（NewErrorReporter，挂在 router 统一错误出口）、事件脱敏
// （RedactBeforeSend，经 logx.Redact 收口）、panic 兜底信封（Recovery，
// 消灭「200 空 body / 500 空 body」两种非信封形态）以及错误编号生成
// （NewErrorID，格式与 R-97 对齐；R-97 落地前 panic 路径由本包临时生成）。
//
// 未配置 SENTRY_DSN 时 main.go 不会初始化 Sentry，也不会挂载本包的任何
// 中间件与上报器——「未配置即无副作用」由装配处保证。
package monitor

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"data-insights/internal/logx"
	"data-insights/internal/middleware"
	"data-insights/internal/response"
	"data-insights/internal/router"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// errorIDGinKey 是 error_id 在 gin context 里的键。ScopeTags 生成并写入，
// Recovery 读取它写进 panic 信封——键只在包内流转，不对外暴露。
const errorIDGinKey = "monitorErrorID"

// NewErrorID 生成 `DI-<biz_code>-<8位hex>` 格式的错误编号（R-97 的稳定格式）。
// R-97（issue #222）落地后应把生成点统一收到 handler 边界；当前 panic 路径
// 先用本实现兜底，保证 panic 信封能带上可反查的编号。
func NewErrorID(bizCode int) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败极罕见；退化成时间戳，编号仍保证唯一性语义。
		return fmt.Sprintf("DI-%d-%08X", bizCode, time.Now().UnixNano()%0xFFFFFFFF)
	}
	return fmt.Sprintf("DI-%d-%s", bizCode, strings.ToUpper(hex.EncodeToString(b)))
}

// RedactBeforeSend 是 Sentry ClientOptions.BeforeSend：事件出站前把 message、
// exception 值、线程值、contexts/extra 类字符串与 breadcrumb 里的机密经
// logx.Redact 抹掉。返回原事件指针（就地改写），调用方（sentry client）继续
// 走采样与发送。
func RedactBeforeSend(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}
	event.Message = logx.Redact(event.Message)
	for i := range event.Exception {
		event.Exception[i].Value = logx.Redact(event.Exception[i].Value)
	}
	for _, b := range event.Breadcrumbs {
		b.Message = logx.Redact(b.Message)
		for k, v := range b.Data {
			if s, ok := v.(string); ok {
				b.Data[k] = logx.Redact(s)
			}
		}
	}
	for name, ctx := range event.Contexts {
		for k, v := range ctx {
			if s, ok := v.(string); ok {
				ctx[k] = logx.Redact(s)
			}
		}
		event.Contexts[name] = ctx
	}
	return event
}

// ScopeTags 在 sentrygin 已建好请求 hub 之后运行：给请求生成 error_id（仅
// panic 时写进信封）、把 request_id / error_id / route 打到 hub scope 上，
// 使 panic 与业务错误两类事件都带同一套关联 tag。必须挂在 newSentryginMiddleware
// 之后（依赖其设置的请求上下文 hub）。
func ScopeTags() gin.HandlerFunc {
	return func(c *gin.Context) {
		errorID := NewErrorID(response.CodeInternalError)
		c.Set(errorIDGinKey, errorID)
		// hub 由 sentrygin 放在请求上下文（sentry.SetHubOnContext）；读它而不是
		// sentrygin.GetHubFromContext（后者读的是 sentrygin 自己从不写入的 gin 键）。
		if hub := sentry.GetHubFromContext(c.Request.Context()); hub != nil {
			hub.Scope().SetTag("request_id", c.Request.Header.Get("X-Request-ID"))
			hub.Scope().SetTag("error_id", errorID)
			if route := c.FullPath(); route != "" {
				hub.Scope().SetTag("route", route)
			}
		}
		c.Next()
	}
}

// Recovery 是最外层的 panic 兜底：sentrygin 以 Repanic: true 把已捕获事件的
// panic 重新抛出后由这里接住，返回带 error_id 的标准信封（HTTP 200 + code
// 50000，与全站信封口径一致）。sentry 未激活时同样兜底，只是没有事件上报。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				errorID := c.GetString(errorIDGinKey)
				if errorID == "" {
					errorID = NewErrorID(response.CodeInternalError)
				}
				slog.Error("panic recovered",
					"error_id", errorID,
					"request_id", c.Request.Header.Get("X-Request-ID"),
					"route", c.FullPath(),
					"panic", logx.Redact(fmt.Sprint(r)))
				response.ErrorWithID(c, response.CodeInternalError, "Internal server error", errorID)
			}
		}()
		c.Next()
	}
}

// NewErrorReporter 返回挂在 router 统一错误出口的上报器：对 50000（服务端
// 内部错误）与 20500（第三方服务错误）调 CaptureException，其余业务码不上报
// （4xx 只进日志，见 R-98 验收 4）。hub 取自 sentrygin 的请求上下文；请求
// 未经过 sentrygin（或 Sentry 未激活）时静默跳过——未配置即无副作用。
func NewErrorReporter() router.ErrorReporter {
	return func(c *gin.Context, code int, err error) {
		if code != response.CodeInternalError && code != response.CodeThirdPartyErr {
			return
		}
		hub := sentry.GetHubFromContext(c.Request.Context())
		if hub == nil {
			return
		}
		hub.Scope().SetTag("biz_code", fmt.Sprintf("%d", code))
		if p, ok := middleware.PrincipalFrom(c); ok {
			hub.Scope().SetTag("user_id", fmt.Sprintf("%d", p.UserID))
		}
		hub.CaptureException(err)
	}
}
