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
