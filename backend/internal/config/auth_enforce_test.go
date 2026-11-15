package config

import "testing"

// TestLoadAuthEnforce 验证 AUTH_ENFORCE 三态解析：默认 off、合法值原样、大小写归一、非法回落 off。
func TestLoadAuthEnforce(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"", "off"},
		{"off", "off"},
		{"shadow", "shadow"},
		{"on", "on"},
		{"ON", "on"},
		{"Shadow", "shadow"},
		{"bogus", "off"},
	}
	for _, c := range cases {
		clearEnv(t)
		if c.raw != "" {
			t.Setenv("AUTH_ENFORCE", c.raw)
		}
		cfg := &Config{}
		if err := cfg.Load(); err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Auth.Enforce != c.want {
			t.Errorf("AUTH_ENFORCE=%q: got %q want %q", c.raw, cfg.Auth.Enforce, c.want)
		}
	}
}
