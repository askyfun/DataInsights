// Package logx 提供跨模块共享的日志基础设施：错误文本脱敏（Redact）与
// LOG_LEVEL 解析（ParseLevel）。R-96（issue #221）的脱敏基线要求「所有写日志
// 与写响应的错误文本经同一个 helper 收口」——本包就是那个唯一的 helper。
package logx

import (
	"log/slog"
	"regexp"
	"strings"
)

// redactRule 是一条「匹配即替换」的脱敏规则。保留可检索的前缀（如
// di_pat_、sk-），把密文本身换成 <redacted>，让日志仍能按类型 grep。
type redactRule struct {
	re   *regexp.Regexp
	repl string
}

var redactRules = []redactRule{
	// URL userinfo：postgres://user:pw@host → postgres://user:<redacted>@host。
	// 密码段要求非空且不含空白/@/斜杠，避免误伤无密码的 user:@host。
	{regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)([^@/\s:]+):([^@/\s]+)@`), `${1}${2}:<redacted>@`},
	// password=xxx / token=xxx（query 串与错误文本两种形态）。
	{regexp.MustCompile(`(?i)\b(password|token)=([^\s&"']+)`), `${1}=<redacted>`},
	// Authorization: Bearer <jwt/pat>。
	{regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)([A-Za-z0-9._-]+)`), `${1}<redacted>`},
	// 平台 PAT（di_pat_ 前缀）与 OpenAI 风格密钥（sk- 前缀）：保留前缀、抹掉密文体。
	{regexp.MustCompile(`di_pat_[A-Za-z0-9_-]+`), `di_pat_<redacted>`},
	{regexp.MustCompile(`sk-[A-Za-z0-9]+`), `sk-<redacted>`},
}

// Redact 抹掉错误文本里出现过的机密：连接串密码、password=/token= 赋值、
// Bearer 令牌、PAT 与第三方 API key。所有写日志与写响应的错误文本都必须经它
// 收口，禁止各处手写替换。
func Redact(s string) string {
	if s == "" {
		return s
	}
	for _, r := range redactRules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

// ParseLevel 把 LOG_LEVEL 原始值解析为 slog 级别，大小写不敏感；
// 空串等同 info。第二个返回值为 false 表示非法值（调用方应回落 info 并 warn 一次）。
func ParseLevel(v string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return slog.LevelInfo, true
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}
