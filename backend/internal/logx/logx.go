// Package logx 提供跨模块共享的错误文本脱敏 helper（Redact）。R-96（issue #221）
// 的脱敏基线要求「所有写日志与写监控的错误文本经同一个 helper 收口」——本包就是
// 那个唯一的 helper。完整包（含 LOG_LEVEL 解析）由 R-96 交付；R-98（issue #223）
// 的 Sentry BeforeSend 先行依赖 Redact，故在此先落脱敏半边，规则与 R-96 保持逐字一致。
package logx

import (
	"regexp"
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
	// 裸 Bearer 令牌（错误文本常丢掉 Authorization: 前缀，只剩 "Bearer <tok>"）。
	// 阈值 10 字符防误伤普通词；授权行已被上一条处理后剩余 "<redacted>"（8 字符）
	// 不会再被本条命中。
	{regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9._-]{10,})`), `bearer <redacted>`},
	// 平台 PAT（di_pat_ 前缀）与 OpenAI 风格密钥（sk- 前缀）：保留前缀、抹掉密文体。
	{regexp.MustCompile(`di_pat_[A-Za-z0-9_-]+`), `di_pat_<redacted>`},
	{regexp.MustCompile(`sk-[A-Za-z0-9]+`), `sk-<redacted>`},
}

// Redact 抹掉错误文本里出现过的机密：连接串密码、password=/token= 赋值、
// Bearer 令牌、PAT 与第三方 API key。所有写日志与写监控的错误文本都必须经它
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
