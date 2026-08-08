// Package config provides Whale homeserver configuration loading,
// validation, and defaults.
package config

import (
	"fmt"
	"os"

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
	Server     Server          `yaml:"server"`
	Database   storage.Config  `yaml:"database"`
	Federation Federation      `yaml:"federation"`
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
	return home + "/.whale/config.yaml"
}
