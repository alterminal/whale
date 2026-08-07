package storage

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config holds database connection parameters.
type Config struct {
	Driver string `yaml:"driver"` // "postgres" or "sqlite"
	DSN    string `yaml:"dsn"`
}

// OpenDB creates and configures a GORM database connection, runs migrations,
// and returns the ready-to-use *gorm.DB instance.
//
// Supported drivers:
//   - "postgres" (uses pgx under the hood via gorm.io/driver/postgres)
//   - "sqlite"  (uses gorm.io/driver/sqlite; DSN is the file path, ":memory:" for in-memory)
//
// For production, prefer explicit SQL migrations over AutoMigrate.
func OpenDB(cfg Config) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.Driver {
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("storage: unsupported driver %q", cfg.Driver)
	}

	logLevel := logger.Info
	if env := os.Getenv("WHALE_LOG_LEVEL"); env == "warn" || env == "error" {
		logLevel = logger.Warn
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.New(
			log.New(os.Stderr, "[gorm] ", log.LstdFlags),
			logger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  logLevel,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: failed to connect to %s: %w", cfg.Driver, err)
	}

	// Connection pool tuning — only for Postgres (SQLite uses single writer).
	if cfg.Driver == "postgres" {
		sqlDB, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("storage: failed to get underlying *sql.DB: %w", err)
		}
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)
		sqlDB.SetConnMaxIdleTime(2 * time.Minute)
	}

	if err := AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("storage: auto-migration failed: %w", err)
	}

	return db, nil
}
