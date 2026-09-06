package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	content := `
Name = "testapp"
Host = "127.0.0.1"
Port = 9090

[Database]
Url = "postgres://user:pass@localhost:5432/testdb?sslmode=disable"
`
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg := &Config{}
	if err := cfg.LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Name != "testapp" {
		t.Errorf("expected name 'testapp', got '%s'", cfg.Name)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected host '127.0.0.1', got '%s'", cfg.Host)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.Database.Url != "postgres://user:pass@localhost:5432/testdb?sslmode=disable" {
		t.Errorf("expected database url, got '%s'", cfg.Database.Url)
	}
}

// TestLoadConfigEnvOverride 验证 DATARAY_SECURITY_KEY 环境变量在 config.toml
// 之后生效（覆盖文件里的 [Security].SecurityKey），未设置时保留文件值。
func TestLoadConfigEnvOverride(t *testing.T) {
	content := `
Name = "testapp"
Host = "127.0.0.1"
Port = 9090

[Database]
Url = "postgres://user:pass@localhost:5432/testdb?sslmode=disable"

[Security]
SecurityKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
`
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	t.Run("env overrides file", func(t *testing.T) {
		t.Setenv("DATARAY_SECURITY_KEY", "ffff0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1")
		cfg := &Config{}
		if err := cfg.LoadConfig(tmpFile.Name()); err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if cfg.Security.SecurityKey != "ffff0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1" {
			t.Errorf("expected env value, got %q", cfg.Security.SecurityKey)
		}
	})

	t.Run("file value kept without env", func(t *testing.T) {
		t.Setenv("DATARAY_SECURITY_KEY", "")
		cfg := &Config{}
		if err := cfg.LoadConfig(tmpFile.Name()); err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if cfg.Security.SecurityKey != "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f" {
			t.Errorf("expected file value, got %q", cfg.Security.SecurityKey)
		}
	})
}

// TestLoadConfigSentryAndCORS 验证 [Sentry].Dsn 与 [CORS].AllowedOrigins 的解析，
// 以及 DATARAY_SENTRY_DSN / DATARAY_CORS_ALLOWED_ORIGINS 环境变量覆盖。
func TestLoadConfigSentryAndCORS(t *testing.T) {
	content := `
Name = "testapp"
Host = "127.0.0.1"
Port = 9090

[Sentry]
Dsn = "https://examplePublicKey@o0.ingest.sentry.io/0"

[CORS]
AllowedOrigins = ["http://localhost:3000"]
`
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	t.Run("parses file values", func(t *testing.T) {
		t.Setenv("DATARAY_SENTRY_DSN", "")
		t.Setenv("DATARAY_CORS_ALLOWED_ORIGINS", "")
		cfg := &Config{}
		if err := cfg.LoadConfig(tmpFile.Name()); err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if cfg.Sentry.Dsn != "https://examplePublicKey@o0.ingest.sentry.io/0" {
			t.Errorf("expected sentry dsn from file, got %q", cfg.Sentry.Dsn)
		}
		if len(cfg.CORS.AllowedOrigins) != 1 || cfg.CORS.AllowedOrigins[0] != "http://localhost:3000" {
			t.Errorf("expected cors origins from file, got %v", cfg.CORS.AllowedOrigins)
		}
	})

	t.Run("env overrides file", func(t *testing.T) {
		t.Setenv("DATARAY_SENTRY_DSN", "https://env@o0.ingest.sentry.io/1")
		t.Setenv("DATARAY_CORS_ALLOWED_ORIGINS", "https://a.example, https://b.example")
		cfg := &Config{}
		if err := cfg.LoadConfig(tmpFile.Name()); err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if cfg.Sentry.Dsn != "https://env@o0.ingest.sentry.io/1" {
			t.Errorf("expected sentry dsn from env, got %q", cfg.Sentry.Dsn)
		}
		if len(cfg.CORS.AllowedOrigins) != 2 || cfg.CORS.AllowedOrigins[0] != "https://a.example" || cfg.CORS.AllowedOrigins[1] != "https://b.example" {
			t.Errorf("expected cors origins from env, got %v", cfg.CORS.AllowedOrigins)
		}
	})
}

func TestLoadConfigNotFound(t *testing.T) {
	cfg := &Config{}
	err := cfg.LoadConfig("/nonexistent/path/config.toml")
	if err == nil {
		t.Error("expected error for nonexistent config file")
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	content := `
Name = "testapp"
Host = "127.0.0.1"
Port = 9090

[Database]
Url = "invalid url"
`
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg := &Config{}
	if err := cfg.LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
}
