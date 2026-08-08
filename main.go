package main

import (
	"fmt"
	"log"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"whale/client"
	"whale/config"
	"whale/storage"
)

func main() {
	fmt.Println("🐋 Whale — Matrix homeserver")

	// -------------------------------------------------------------------
	// Configuration loading order (last loaded wins):
	//   1. Hardcoded defaults
	//   2. Environment variables     (lowest priority)
	//   3. ~/.config/whale/config.yaml
	//   4. .env file                (highest priority)
	// -------------------------------------------------------------------
	cfg := config.Default()

	// Layer 2: environment variables
	if dsn := os.Getenv("WHALE_DATABASE_DSN"); dsn != "" {
		cfg.Database.DSN = dsn
	}
	if driver := os.Getenv("WHALE_DATABASE_DRIVER"); driver != "" {
		cfg.Database.Driver = driver
	}
	if name := os.Getenv("WHALE_SERVER_NAME"); name != "" {
		cfg.Server.Name = name
	}
	if domain := os.Getenv("WHALE_SERVER_DOMAIN"); domain != "" {
		cfg.Server.Domain = domain
	}

	// Layer 3: YAML config file
	if yamlCfg, err := config.LoadYAML(config.DefaultPath()); err == nil {
		cfg = config.MergeYAML(cfg, yamlCfg)
		fmt.Printf("📄 Loaded config: %s\n", config.DefaultPath())
	} else {
		fmt.Printf("ℹ️  No config file at %s (using env/defaults)\n", config.DefaultPath())
	}

	// Layer 4: .env file (highest priority)
	if envMap, err := config.LoadDotEnv(".env"); err == nil {
		cfg.ApplyDotEnv(envMap)
		fmt.Println("📄 Loaded .env (highest priority)")
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	// Open database
	db, err := storage.OpenDB(cfg.Database)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	// Set up Echo server
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// -------------------------------------------------------------------
	// Global middleware
	// -------------------------------------------------------------------
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "${method} ${uri} ${status} ${latency_human}\n",
	}))
	e.Use(middleware.Recover())

	// -------------------------------------------------------------------
	// Client-Server API handler
	// -------------------------------------------------------------------
	h := client.NewHandler(db, cfg.Server.Name, cfg.Server.Domain)

	// Configure .well-known discovery
	h.WellKnownCfg = client.WellKnownConfig{
		BaseURL:        os.Getenv("WHALE_BASE_URL"),        // e.g., "https://matrix.example.com"
		FederationHost: os.Getenv("WHALE_FEDERATION_HOST"), // e.g., "matrix.example.com:8448"
		FederationPort: cfg.Federation.Port,
	}

	// -------------------------------------------------------------------
	// .well-known endpoints — must be at domain root per RFC 8615
	// -------------------------------------------------------------------
	h.RegisterWellKnown(e)
	// CORS preflight for well-known (browsers do cross-origin fetches)
	e.OPTIONS("/.well-known/matrix/*", h.ServeWellKnownOPTIONS)

	// -------------------------------------------------------------------
	// Matrix Client-Server API under /_matrix
	// -------------------------------------------------------------------
	api := e.Group("/_matrix")
	h.RegisterRoutes(api)

	// Health check
	e.GET("/health", func(c echo.Context) error {
		return c.String(200, "OK")
	})

	// -------------------------------------------------------------------
	// Print startup banner
	// -------------------------------------------------------------------
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	fmt.Printf("✅ Whale homeserver starting on %s\n", addr)
	fmt.Printf("   Server name : %s\n", cfg.Server.Name)
	fmt.Printf("   Database    : %s\n", cfg.Database.Driver)
	fmt.Println()
	fmt.Printf("   📡 .well-known endpoints:\n")
	printWellKnown("client", h.WellKnownCfg, cfg)
	printWellKnown("server", h.WellKnownCfg, cfg)
	fmt.Println()

	// Federation server (separate port) — TODO
	if cfg.Federation.Enabled {
		fmt.Printf("🌐 Federation will listen on port %d (not yet started)\n", cfg.Federation.Port)
	}

	e.Logger.Fatal(e.Start(addr))
}

// printWellKnown logs the effective .well-known URL for documentation.
func printWellKnown(kind string, wkCfg client.WellKnownConfig, cfg config.Config) {
	base := fmt.Sprintf("http://%s:%d", cfg.Server.Name, cfg.Server.Port)
	url := base + "/.well-known/matrix/" + kind

	var target string
	switch kind {
	case "client":
		target = wkCfg.BaseURL
		if target == "" {
			target = fmt.Sprintf("http://%s:8008", cfg.Server.Name)
		}
	case "server":
		target = wkCfg.FederationHost
		if target == "" {
			target = fmt.Sprintf("%s:%d", cfg.Server.Name, cfg.Federation.Port)
		}
	}
	fmt.Printf("      %-6s %s → %s\n", kind, url, target)
}
