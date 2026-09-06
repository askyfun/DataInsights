package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Name     string         `toml:"Name"`
	Host     string         `toml:"Host"`
	Port     int            `toml:"Port"`
	Database DatabaseConfig `toml:"Database"`
	Security SecurityConfig `toml:"Security"`
}

type DatabaseConfig struct {
	Url string `toml:"Url"`
}

type SecurityConfig struct {
	SecurityKey string `toml:"SecurityKey"`
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

	return nil
}
