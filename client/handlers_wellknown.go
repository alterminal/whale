package client

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// =========================================================================
// GET /.well-known/matrix/client
// =========================================================================
//
// Client discovery endpoint per:
//
//	https://spec.matrix.org/latest/client-server-api/#getwell-knownmatrixclient
//
// This is fetched by clients when a user enters a Matrix ID like
// @bob:example.com. The client performs a GET to
// https://example.com/.well-known/matrix/client to discover the actual
// homeserver base URL.
//
// Response format:
//
//	{
//	  "m.homeserver": {
//	    "base_url": "https://matrix.example.com"
//	  },
//	  "m.identity_server": {
//	    "base_url": "https://identity.example.com"
//	  },
//	  "org.example.custom": {
//	    ...
//	  }
//	}
//
// Only "m.homeserver" is required. "m.identity_server" is optional and
// custom keys are permitted but must use Java-style reversed domain names.
//
// # Cacheability
//
// Responses include Cache-Control: public, max-age=3600. Clients should
// cache the result per the spec to avoid excess traffic on user lookups.
func (h *Handler) WellKnownClient(c echo.Context) error {
	baseURL := h.resolveClientBaseURL(c)

	resp := map[string]interface{}{
		"m.homeserver": map[string]interface{}{
			"base_url": baseURL,
		},
	}

	if h.WellKnownCfg.IdentityServer != "" {
		resp["m.identity_server"] = map[string]interface{}{
			"base_url": h.WellKnownCfg.IdentityServer,
		}
	}

	return wellKnownJSON(c, resp)
}

// =========================================================================
// GET /.well-known/matrix/server
// =========================================================================
//
// Server discovery endpoint per:
//
//	https://spec.matrix.org/latest/server-server-api/#getwell-knownmatrixserver
//
// Fetched by other homeservers to discover how to reach this server for
// federation. When a server needs to send an event to @alice:example.com,
// it first resolves example.com via this endpoint.
//
// Response format:
//
//	{
//	  "m.server": "matrix.example.com:8448"
//	}
//
// The value is a host:port pair. SRV delegation via DNS is also specified
// but not implemented here — well-known takes precedence over SRV records
// per the Matrix spec hierarchy:
//
//	1. well-known (this endpoint)
//	2. SRV record (_matrix._tcp.<server_name>)
//	3. Default: <server_name>:8448
//
// Cache-Control is set aggressively (1 day) since federation targets
// rarely change.
func (h *Handler) WellKnownServer(c echo.Context) error {
	fedTarget := h.resolveFederationHost()

	resp := map[string]interface{}{
		"m.server": fedTarget,
	}

	return wellKnownJSON(c, resp)
}

// =========================================================================
// Internal helpers
// =========================================================================

// resolveClientBaseURL determines the homeserver base URL for the client
// well-known response. Priority:
//
//  1. WellKnownCfg.BaseURL (explicit config)
//  2. Constructed from the request's scheme + ServerName + port
//
// In production, BaseURL should always be set explicitly to the correct
// public URL (e.g., "https://matrix.example.com").
func (h *Handler) resolveClientBaseURL(c echo.Context) string {
	if h.WellKnownCfg.BaseURL != "" {
		return h.WellKnownCfg.BaseURL
	}

	// Auto-detect from the incoming request.
	scheme := "http"
	if c.IsTLS() {
		scheme = "https"
	}
	// Respect X-Forwarded-Proto if behind a reverse proxy.
	if fwdProto := c.Request().Header.Get("X-Forwarded-Proto"); fwdProto != "" {
		scheme = fwdProto
	}

	host := c.Request().Host
	// Extract host without port (the well-known is served at the apex domain)
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}

	// Use the server_name as the effective host for the Matrix API.
	// This is correct when the well-known domain differs from the
	// homeserver domain (delegation).
	targetHost := h.ServerName

	// If the request is already hitting the homeserver directly
	// (i.e., the well-known domain == server_name), include the port.
	port := c.Request().URL.Port()
	if port == "" {
		// Default Matrix C-S port
		port = "8008"
	}

	return fmt.Sprintf("%s://%s:%s", scheme, targetHost, port)
}

// resolveFederationHost determines the federation target for the server
// well-known response. Priority:
//
//  1. WellKnownCfg.FederationHost (explicit, e.g., "matrix.example.com:8448")
//  2. Constructed from ServerName + FederationPort
//  3. ServerName + default federation port (8448)
func (h *Handler) resolveFederationHost() string {
	if h.WellKnownCfg.FederationHost != "" {
		return h.WellKnownCfg.FederationHost
	}

	port := h.WellKnownCfg.FederationPort
	if port == 0 {
		port = 8448
	}

	return fmt.Sprintf("%s:%d", h.ServerName, port)
}

// wellKnownJSON sends a JSON response with proper caching headers for
// well-known discovery endpoints.
//
// Headers set:
//   - Content-Type: application/json
//   - Access-Control-Allow-Origin: * (clients may fetch from any origin)
//   - Cache-Control: public, max-age=N (cacheable per spec)
func wellKnownJSON(c echo.Context, payload interface{}) error {
	c.Response().Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Response().Header().Set("Access-Control-Allow-Headers", "Content-Type")
	c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", wellKnownMaxAge(c.Request().URL.Path)))

	return c.JSON(http.StatusOK, payload)
}

// wellKnownMaxAge returns the appropriate Cache-Control max-age for each
// well-known endpoint. Server targets change very rarely; client targets
// more frequently.
func wellKnownMaxAge(path string) int {
	if strings.Contains(path, "/server") {
		// Federation targets are effectively static — 24 hours.
		return int((24 * time.Hour).Seconds())
	}
	// Client discovery — 1 hour (spec recommendation).
	return int((1 * time.Hour).Seconds())
}

// ServeWellKnownOPTIONS handles CORS preflight for /.well-known/matrix/*.
//
// This is registered separately to handle OPTIONS requests without body
// parsing overhead. Browsers send preflight before cross-origin well-known
// fetches.
func (h *Handler) ServeWellKnownOPTIONS(c echo.Context) error {
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Response().Header().Set("Access-Control-Allow-Headers", "Content-Type")
	c.Response().Header().Set("Access-Control-Max-Age", "86400")
	return c.NoContent(http.StatusNoContent)
}
