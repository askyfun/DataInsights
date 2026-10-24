package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"

	"data-insights/internal/response"

	"github.com/gin-gonic/gin"
)

// requestIDPattern 校验入站 X-Request-ID：仅 [A-Za-z0-9_-]，上限 128 字符。
// 超长串与怪字符会原样进日志与响应头，必须挡在门口（R-96 验收 6）。
var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// RequestID 返回请求 id 中间件：采信合法的入站 X-Request-ID，缺失或非法时
// 服务端重新生成（不透传、不报错）。id 写入 gin context 键 response.RequestIDKey
// ——这是 response.getTraceID 与日志链路的**唯一**读取口径，不依赖任何
// 「把 id 写回入站头」的副作用（那条隐性依赖已在 R-96 移除）。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if !requestIDPattern.MatchString(requestID) {
			requestID = newRequestID()
		}
		c.Set(response.RequestIDKey, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// newRequestID 生成 32 位 hex 的随机请求 id；crypto/rand 失败时（几乎不可能）
// 退化为纳秒时间戳，保证日志链路永远有 id 可用。
func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
