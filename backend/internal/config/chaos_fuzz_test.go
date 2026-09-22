package config

// fuzz 回归探针（chaos 测试转正，2026-09-20）：钉住配置解析的健壮性不变式。

import (
	"os"
	"testing"
)

// FuzzZZPortEnv 钉住 PORT 解析：要么报错，要么落在 [1,65535]，绝不静默接受非法值。
func FuzzZZPortEnv(f *testing.F) {
	for _, s := range []string{
		"", "0", "1", "65535", "65536", "-1", "-0", "+1", " 80", "80 ",
		"8_0", "0x50", "1e3", "23352", "999999999999999999999999",
		"abc", "８０", "80\n", "80;DROP",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		prev, had := os.LookupEnv("PORT")
		defer func() {
			if had {
				os.Setenv("PORT", prev)
			} else {
				os.Unsetenv("PORT")
			}
		}()
		if err := os.Setenv("PORT", in); err != nil {
			t.Skip("环境变量不可写")
		}

		var c Config
		err := c.Load()
		if err != nil {
			// 报错即可：调用方 fail-fast 丢弃整个 Config（部分写入的字段无意义）。
			return
		}
		if c.Host != ListenHost {
			t.Fatalf("Host 被 PORT=%q 影响：%q", in, c.Host)
		}
		if in == "" {
			if c.Port != defaultPort {
				t.Fatalf("PORT 为空时未用默认值：got %d want %d", c.Port, defaultPort)
			}
			return
		}
		if c.Port < 1 || c.Port > 65535 {
			t.Fatalf("PORT=%q 被静默接受为 %d（越界）", in, c.Port)
		}
	})
}
