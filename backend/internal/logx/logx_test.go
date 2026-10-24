package logx

import (
	"log/slog"
	"testing"
)

// TestRedact 表驱动覆盖 R-96 验收 5 的脱敏基线：
// 连接串密码、password=/token=、Bearer、PAT、API key，以及不含机密的文本原样通过。
func TestRedact(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"pg 连接串密码", `failed to connect: postgres://app:secret123@10.0.0.12:5432/db`, `failed to connect: postgres://app:<redacted>@10.0.0.12:5432/db`},
		{"mysql 连接串密码", `dial mysql://root:pw123@127.0.0.1:3306/x`, `dial mysql://root:<redacted>@127.0.0.1:3306/x`},
		{"无密码连接串不动", `postgres://app@10.0.0.12:5432/db`, `postgres://app@10.0.0.12:5432/db`},
		{"password= 赋值", `pq: auth failed user=app password=topsecret`, `pq: auth failed user=app password=<redacted>`},
		{"PASSWORD= 大小写", `PASSWORD=abc host=db`, `PASSWORD=<redacted> host=db`},
		{"password= 在 query 串", `?sslmode=disable&password=s3cret&port=5432`, `?sslmode=disable&password=<redacted>&port=5432`},
		{"token= 赋值", `token=abcdef123 and more`, `token=<redacted> and more`},
		{"token= 在 query 串", `?page=1&token=zzz9`, `?page=1&token=<redacted>`},
		{"Bearer 令牌", `Authorization: Bearer eyJhbGciOi.abc-123`, `Authorization: Bearer <redacted>`},
		{"bearer 小写", `authorization: bearer di_pat_x1y2z3`, `authorization: bearer <redacted>`},
		{"平台 PAT", `verify failed for di_pat_9f8e7d6c5b`, `verify failed for di_pat_<redacted>`},
		{"sk- 密钥", `upstream 401 for sk-Ab3xY91234`, `upstream 401 for sk-<redacted>`},
		{"同一文本多处机密", `postgres://u:p@h/db token=tk1 di_pat_longtoken99`, `postgres://u:<redacted>@h/db token=<redacted> di_pat_<redacted>`},
		{"普通错误文本原样", `dial tcp 10.0.0.12:5432: connection refused`, `dial tcp 10.0.0.12:5432: connection refused`},
		{"空串原样", "", ""},
		{"authored 词不被误伤（passwordX=）", `passwordx=keep`, `passwordx=keep`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Redact(tc.in); got != tc.want {
				t.Fatalf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
		ok   bool
	}{
		{"", slog.LevelInfo, true},
		{"debug", slog.LevelDebug, true},
		{"INFO", slog.LevelInfo, true},
		{"warn", slog.LevelWarn, true},
		{"warning", slog.LevelWarn, true},
		{"Error", slog.LevelError, true},
		{" info ", slog.LevelInfo, true},
		{"verbose", slog.LevelInfo, false},
		{"2", slog.LevelInfo, false},
	}
	for _, tc := range cases {
		got, ok := ParseLevel(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("ParseLevel(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
