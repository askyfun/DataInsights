package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Name     string         `toml:"Name"`
	Host     string         `toml:"Host"`
	Port     int            `toml:"Port"`
	Database DatabaseConfig `toml:"Database"`
	Security SecurityConfig `toml:"Security"`
	Sentry   SentryConfig   `toml:"Sentry"`
	CORS     CORSConfig     `toml:"CORS"`
}

type DatabaseConfig struct {
	Url string `toml:"Url"`
}

type SecurityConfig struct {
	SecurityKey string `toml:"SecurityKey"`
}

type SentryConfig struct {
	Dsn string `toml:"Dsn"`
}

type CORSConfig struct {
	AllowedOrigins []string `toml:"AllowedOrigins"`
}

func (c *Config) LoadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := toml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if v := os.Getenv("DATARAY_SECURITY_KEY"); v != "" {
		c.Security.SecurityKey = v
	}

	if v := os.Getenv("DATARAY_SENTRY_DSN"); v != "" {
		c.Sentry.Dsn = v
	}

	if v := os.Getenv("DATARAY_CORS_ALLOWED_ORIGINS"); v != "" {
		origins := strings.Split(v, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		c.CORS.AllowedOrigins = origins
	}

	return nil
}
