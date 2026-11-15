package config

import "testing"

// TestLoadAuthEnforce 验证 AUTH_ENFORCE 的解析与「未设置时按环境推导」：
//   - 显式值永远优先（合法值原样、大小写归一）；
//   - 非法值回落**环境默认**（不再是恒定 off）；
//   - 未设置时：生产 / 未设置 / 认不出的取值（staging、私有云名字）一律 on，
//     只有本地开发族（development/dev/local/test）才是 off。
//
// 这条默认值的动机：默认 off 意味着「装完就裸奔」，只有读过文档的人才会去开；
// 默认 on 时最坏结果是本地开发被打断（显式写 off 即可），取舍明显更划算。
func TestLoadAuthEnforce(t *testing.T) {
	explicit := []struct {
		appEnv string
		raw    string
		want   string
	}{
		{"production", "off", "off"},
		{"production", "shadow", "shadow"},
		{"production", "on", "on"},
		{"development", "ON", "on"},
		{"development", "Shadow", "shadow"},
		{"development", "bogus", "off"}, // 非法值 → 回落环境默认（development → off）
		{"production", "bogus", "on"},   // 非法值 → 回落环境默认（production → on）
	}
	for _, c := range explicit {
		clearEnv(t)
		t.Setenv("APP_ENV", c.appEnv)
		t.Setenv("AUTH_ENFORCE", c.raw)
		cfg := &Config{}
		if err := cfg.Load(); err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Auth.Enforce != c.want {
			t.Errorf("APP_ENV=%q AUTH_ENFORCE=%q: got %q want %q",
				c.appEnv, c.raw, cfg.Auth.Enforce, c.want)
		}
	}

	implicit := []struct {
		appEnv string
		want   string
	}{
		{"", "on"}, // 未设置 APP_ENV → production（defaultAppEnv）
		{"production", "on"},
		{"prod", "on"},
		{"staging", "on"}, // 不在开发白名单里，按生产对待
		{"development", "off"},
		{"dev", "off"},
		{"local", "off"},
		{"TEST", "off"},
	}
	for _, c := range implicit {
		clearEnv(t)
		if c.appEnv != "" {
			t.Setenv("APP_ENV", c.appEnv)
		}
		cfg := &Config{}
		if err := cfg.Load(); err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Auth.Enforce != c.want {
			t.Errorf("APP_ENV=%q (AUTH_ENFORCE unset): got %q want %q", c.appEnv, cfg.Auth.Enforce, c.want)
		}
	}
}

// TestIsDevEnv 把白名单本身钉住：只有明确认识的开发环境才允许默认关闭鉴权。
// 新增/删除条目必须同时改 defaultAuthEnforceFor 的语义说明（文档 configuration.md）。
func TestIsDevEnv(t *testing.T) {
	for _, env := range []string{"development", "dev", "local", "test", "testing", " DEV ", "Local"} {
		if !isDevEnv(env) {
			t.Errorf("%q should be a dev env", env)
		}
	}
	for _, env := range []string{"", "production", "prod", "staging", "uat", "demo", "私有云"} {
		if isDevEnv(env) {
			t.Errorf("%q must NOT be treated as a dev env", env)
		}
	}
}
