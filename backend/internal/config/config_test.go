package config

import (
	"os"
	"testing"
)

// clearEnv 把全部被识别的环境变量置空。t.Setenv 会在用例结束后自动还原。
// 空字符串等价于"未设置"（见 Load 的语义），因此不需要 os.Unsetenv。
//
// ⚠️ 不能与 LoadDotEnv 混用：置空只是把变量设成"存在但为空"，而 godotenv 的语义是
// "已存在的变量一律不覆盖"，.env 里的值会被这个空壳挡住。要验 .env 装载请用 unsetEnv。
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"PORT", "DATABASE_URL", "SECURITY_KEY", "SENTRY_DSN", "STATIC_DIR", "CORS_ALLOWED_ORIGINS",
	} {
		t.Setenv(name, "")
	}
}

// unsetEnv 真正删除变量（而非置空），用例结束后还原。凡是要经过 godotenv 的用例
// 都必须用它，理由见 clearEnv 的注释。
func unsetEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		old, had := os.LookupEnv(name)
		os.Unsetenv(name)
		t.Cleanup(func() {
			if had {
				os.Setenv(name, old)
				return
			}
			os.Unsetenv(name)
		})
	}
}

// TestLoadDefaults 验证未设置任何环境变量时的内置默认值。
// 连接串没有默认值：Load 只负责留空，是否致命由调用方决定。
func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg := &Config{}
	if err := cfg.Load(); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Host != ListenHost {
		t.Errorf("expected listen host %q, got %q", ListenHost, cfg.Host)
	}
	if cfg.Port != defaultPort {
		t.Errorf("expected default port %d, got %d", defaultPort, cfg.Port)
	}
	if cfg.Database.Url != "" {
		t.Errorf("expected empty database url (no default), got %q", cfg.Database.Url)
	}
	if cfg.Security.SecurityKey != "" || cfg.Sentry.Dsn != "" || len(cfg.CORS.AllowedOrigins) != 0 {
		t.Errorf("expected optional fields to stay empty, got %+v", cfg)
	}
}

// TestLoadFromEnv 验证全字段都能从环境变量装配 —— 这是唯一来源。
func TestLoadFromEnv(t *testing.T) {
	// 监听地址不是配置项：这里显式设一个 HOST，也必须被完全忽略。
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/testdb?sslmode=disable")
	t.Setenv("SECURITY_KEY", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	t.Setenv("SENTRY_DSN", "https://examplePublicKey@o0.ingest.sentry.io/0")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example, https://b.example")

	cfg := &Config{}
	if err := cfg.Load(); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Host != ListenHost {
		t.Errorf("listen host is fixed and must ignore HOST env, got %q", cfg.Host)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected port from env, got %d", cfg.Port)
	}
	if cfg.Database.Url != "postgres://user:pass@localhost:5432/testdb?sslmode=disable" {
		t.Errorf("expected database url from env, got %q", cfg.Database.Url)
	}
	if cfg.Security.SecurityKey != "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f" {
		t.Errorf("expected security key from env, got %q", cfg.Security.SecurityKey)
	}
	if cfg.Sentry.Dsn != "https://examplePublicKey@o0.ingest.sentry.io/0" {
		t.Errorf("expected sentry dsn from env, got %q", cfg.Sentry.Dsn)
	}
	// 逗号分隔，两侧空白必须去掉，否则 Origin 精确匹配会全部落空。
	if len(cfg.CORS.AllowedOrigins) != 2 ||
		cfg.CORS.AllowedOrigins[0] != "https://a.example" ||
		cfg.CORS.AllowedOrigins[1] != "https://b.example" {
		t.Errorf("expected trimmed cors origins, got %v", cfg.CORS.AllowedOrigins)
	}
}

// TestLoadEmptyValuesKeepDefaults 验证空字符串不覆盖默认值：.env 里
// `CORS_ALLOWED_ORIGINS=` 这类留空占位不该把配置打成空。
func TestLoadEmptyValuesKeepDefaults(t *testing.T) {
	clearEnv(t)

	cfg := &Config{}
	if err := cfg.Load(); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Host != ListenHost || cfg.Port != defaultPort {
		t.Errorf("empty env values must not override defaults, got %+v", cfg)
	}
	if len(cfg.CORS.AllowedOrigins) != 0 {
		t.Errorf("expected empty cors allowlist, got %v", cfg.CORS.AllowedOrigins)
	}
}

// TestLoadInvalidPort 验证非法端口在装配阶段就报错，而不是带着 0 或
// 越界值去监听，导致运行时才出现难以定位的失败。
func TestLoadInvalidPort(t *testing.T) {
	for name, value := range map[string]string{
		"not a number": "abc",
		"zero":         "0",
		"negative":     "-1",
		"out of range": "70000",
	} {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("PORT", value)
			cfg := &Config{}
			if err := cfg.Load(); err == nil {
				t.Errorf("expected error for PORT=%q", value)
			}
		})
	}
}

// TestLoadDotEnv 验证 .env 文件加载：格式解析、真实环境变量优先、文件缺失与
// 格式非法的处理。这是"环境变量优先于文件"这条业界规则的落点。
func TestLoadDotEnv(t *testing.T) {
	const key = "TEST_DOTENV_VALUE"
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })

	writeEnvFile := func(t *testing.T, content string) string {
		t.Helper()
		f, err := os.CreateTemp("", "env-*.env")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		t.Cleanup(func() { os.Remove(f.Name()) })
		if _, err := f.WriteString(content); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}
		f.Close()
		return f.Name()
	}

	t.Run("loads file into process env", func(t *testing.T) {
		path := writeEnvFile(t, "# 注释\n"+key+"=from-file\n")
		loaded, err := LoadDotEnv(path)
		if err != nil {
			t.Fatalf("failed to load env file: %v", err)
		}
		if loaded != path {
			t.Errorf("expected loaded path %q, got %q", path, loaded)
		}
		if got := os.Getenv(key); got != "from-file" {
			t.Errorf("expected value from file, got %q", got)
		}
	})

	t.Run("real env wins over file", func(t *testing.T) {
		os.Setenv(key, "from-shell")
		defer os.Unsetenv(key)

		path := writeEnvFile(t, key+"=from-file\n")
		if _, err := LoadDotEnv(path); err != nil {
			t.Fatalf("failed to load env file: %v", err)
		}
		if got := os.Getenv(key); got != "from-shell" {
			t.Errorf("real environment variable must win, got %q", got)
		}
	})

	t.Run("missing file is not an error", func(t *testing.T) {
		loaded, err := LoadDotEnv("/nonexistent/path/.env", "")
		if err != nil {
			t.Fatalf("missing env file must not be fatal, got: %v", err)
		}
		if loaded != "" {
			t.Errorf("expected empty path, got %q", loaded)
		}
	})

	t.Run("first existing candidate wins", func(t *testing.T) {
		os.Unsetenv(key)
		missing := "/nonexistent/path/.env"
		path := writeEnvFile(t, key+"=from-second\n")
		loaded, err := LoadDotEnv(missing, path)
		if err != nil {
			t.Fatalf("failed to load env file: %v", err)
		}
		if loaded != path {
			t.Errorf("expected second candidate %q, got %q", path, loaded)
		}
	})

	t.Run("malformed file is an error", func(t *testing.T) {
		path := writeEnvFile(t, "this line has no separator\n")
		if _, err := LoadDotEnv(path); err == nil {
			t.Error("expected error for malformed env file")
		}
	})
}

// TestLoadReadsDotEnvValues 端到端串起 LoadDotEnv 与 Load：
// .env 装载后可以被 Load 读到，且真实环境变量仍然优先。
func TestLoadReadsDotEnvValues(t *testing.T) {
	unsetEnv(t, "DATABASE_URL")

	f, err := os.CreateTemp("", "env-*.env")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	if _, err := f.WriteString("DATABASE_URL=postgres://dotenv@localhost:5432/dotenvdb?sslmode=disable\n"); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	f.Close()

	if _, err := LoadDotEnv(f.Name()); err != nil {
		t.Fatalf("failed to load env file: %v", err)
	}

	cfg := &Config{}
	if err := cfg.Load(); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Database.Url != "postgres://dotenv@localhost:5432/dotenvdb?sslmode=disable" {
		t.Errorf("expected database url from .env file, got %q", cfg.Database.Url)
	}

	t.Run("real env overrides dotenv", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://shell@localhost:5432/shelldb?sslmode=disable")
		cfg := &Config{}
		if err := cfg.Load(); err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if cfg.Database.Url != "postgres://shell@localhost:5432/shelldb?sslmode=disable" {
			t.Errorf("real env must win, got %q", cfg.Database.Url)
		}
	})
}

// TestEnvVarNamesAreStable 把对外契约（环境变量的名字）钉成字面量。
//
// 这是本文件里唯一一处刻意不引用符号的测试：用常量或变量写的断言抓不到
// "改了变量名但忘了同步读取方"—— 名字本身就是契约，所以必须写死裸字符串。
// 有人把 PORT 改成 APP_PORT 时，这个用例会立刻变红。
func TestEnvVarNamesAreStable(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "18080")
	t.Setenv("DATABASE_URL", "postgres://literal@localhost:5432/literal?sslmode=disable")
	t.Setenv("SECURITY_KEY", "deadbeef")
	t.Setenv("SENTRY_DSN", "https://literal@sentry.io/1")
	t.Setenv("STATIC_DIR", "/tmp/literal-web")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://literal.example")

	cfg := &Config{}
	if err := cfg.Load(); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Port != 18080 {
		t.Errorf("PORT not honored, got port %d", cfg.Port)
	}
	if cfg.Database.Url != "postgres://literal@localhost:5432/literal?sslmode=disable" {
		t.Errorf("DATABASE_URL not honored, got %q", cfg.Database.Url)
	}
	if cfg.Security.SecurityKey != "deadbeef" {
		t.Errorf("SECURITY_KEY not honored, got %q", cfg.Security.SecurityKey)
	}
	if cfg.Sentry.Dsn != "https://literal@sentry.io/1" {
		t.Errorf("SENTRY_DSN not honored, got %q", cfg.Sentry.Dsn)
	}
	if cfg.StaticDir != "/tmp/literal-web" {
		t.Errorf("STATIC_DIR not honored, got %q", cfg.StaticDir)
	}
	if len(cfg.CORS.AllowedOrigins) != 1 || cfg.CORS.AllowedOrigins[0] != "https://literal.example" {
		t.Errorf("CORS_ALLOWED_ORIGINS not honored, got %v", cfg.CORS.AllowedOrigins)
	}
}

// TestListenHostIsNotConfigurable 钉死"监听地址没有 env 口子"：
// 即使有人把 HOST 设回来，也必须被忽略，恒为 ListenHost。
func TestListenHostIsNotConfigurable(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOST", "127.0.0.1")

	cfg := &Config{}
	if err := cfg.Load(); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Host != ListenHost {
		t.Errorf("listen host must stay %q, got %q", ListenHost, cfg.Host)
	}
}
