// Package config provides Whale homeserver configuration loading,
// validation, and defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"whale/storage"
)

// Server holds HTTP listener configuration.
type Server struct {
	Name   string `yaml:"name"`   // server_name (e.g., "matrix.example.com")
	Port   int    `yaml:"port"`   // client-server API port (default 8008)
	Domain string `yaml:"domain"` // domain part of MXIDs (default: server_name)
}

// Federation holds Server-Server API configuration.
type Federation struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"` // federation port (default 8448)
	Bind    string `yaml:"bind"` // bind address (default "0.0.0.0")
}

// Config is the top-level Whale configuration.
type Config struct {
	Server     Server         `yaml:"server"`
	Database   storage.Config `yaml:"database"`
	Federation Federation     `yaml:"federation"`
}

// Default returns a Config with sensible development defaults.
func Default() Config {
	return Config{
		Server: Server{
			Name:   "localhost",
			Port:   8008,
			Domain: "localhost",
		},
		Database: storage.Config{
			Driver: "postgres",
			DSN:    "host=localhost user=whale_dev password=ZFmwJxYISdaJpIuI0VJ8 dbname=whale_dev port=5432 sslmode=disable",
		},
		Federation: Federation{
			Enabled: true,
			Port:    8448,
		},
	}
}

// Validate checks that required fields are present and values are sane.
func (c *Config) Validate() error {
	if c.Server.Name == "" {
		return fmt.Errorf("config: server.name is required")
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8008
	}
	if c.Server.Domain == "" {
		c.Server.Domain = c.Server.Name
	}
	if c.Database.Driver == "" {
		return fmt.Errorf("config: database.driver is required")
	}
	if c.Database.DSN == "" {
		return fmt.Errorf("config: database.dsn is required")
	}

	// Validate DSN format for known drivers
	switch c.Database.Driver {
	case "postgres", "sqlite":
		// ok
	default:
		return fmt.Errorf("config: unsupported database.driver %q (expected 'postgres' or 'sqlite')", c.Database.Driver)
	}

	if c.Federation.Enabled && c.Federation.Port == 0 {
		c.Federation.Port = 8448
	}

	return nil
}

// DefaultPath returns the conventional config file location.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "whale", "config.yaml")
}

// LoadYAML reads and parses a YAML config file. Non-zero fields in the file
// override the defaults; missing fields keep their default values.
func LoadYAML(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config: failed to parse %s: %w", path, err)
	}
	return cfg, nil
}

// MergeYAML overlays non-zero values from src onto dst and returns the result.
// Only fields that are explicitly set (non-zero) in src will override dst.
func MergeYAML(dst, src Config) Config {
	if src.Server.Name != "" {
		dst.Server.Name = src.Server.Name
	}
	if src.Server.Port != 0 {
		dst.Server.Port = src.Server.Port
	}
	if src.Server.Domain != "" {
		dst.Server.Domain = src.Server.Domain
	}
	if src.Database.Driver != "" {
		dst.Database.Driver = src.Database.Driver
	}
	if src.Database.DSN != "" {
		dst.Database.DSN = src.Database.DSN
	}
	if src.Federation.Port != 0 {
		dst.Federation.Port = src.Federation.Port
	}
	// Federation.Enabled: if src has it as false, respect that (only override if src is explicitly true).
	// Since false is the zero value for bool, we can't distinguish "not set" from "set to false".
	// We always take src.Enabled — YAML 'enabled: false' is treated as explicit.
	// To keep it simple: overlay Enabled unconditionally if the YAML was loaded.
	dst.Federation.Enabled = src.Federation.Enabled
	if src.Federation.Bind != "" {
		dst.Federation.Bind = src.Federation.Bind
	}
	return dst
}

// LoadDotEnv parses a .env file and returns key-value pairs.
// Supports:
//   - KEY=VALUE
//   - KEY="VALUE" / KEY='VALUE'
//   - # comments
//   - blank lines
func LoadDotEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Strip surrounding quotes
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			value = value[1 : len(value)-1]
		}
		result[key] = value
	}
	return result, nil
}

// ApplyDotEnv applies values from a .env map onto the Config.
// Supported keys:
//   - WHALE_DATABASE_DSN
//   - WHALE_DATABASE_DRIVER
//   - WHALE_SERVER_NAME
//   - WHALE_SERVER_PORT
//   - WHALE_SERVER_DOMAIN
func (c *Config) ApplyDotEnv(env map[string]string) {
	if v, ok := env["WHALE_DATABASE_DSN"]; ok {
		c.Database.DSN = v
	}
	if v, ok := env["WHALE_DATABASE_DRIVER"]; ok {
		c.Database.Driver = v
	}
	if v, ok := env["WHALE_SERVER_NAME"]; ok {
		c.Server.Name = v
	}
	if v, ok := env["WHALE_SERVER_PORT"]; ok {
		if p, err := strconv.Atoi(v); err == nil {
			c.Server.Port = p
		}
	}
	if v, ok := env["WHALE_SERVER_DOMAIN"]; ok {
		c.Server.Domain = v
	}
}
