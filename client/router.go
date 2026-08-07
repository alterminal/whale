// Package client implements the Matrix Client-Server API (r3.0 / v3).
//
// Route layout:
//
//	/_matrix/client/versions
//	/_matrix/client/v3/
//	  login, register, logout, logout/all
//	  account/whoami
//	  createRoom
//	  directory/room/{roomAlias}
//	  joined_rooms
//	  rooms/{roomId}/
//	    join, leave, invite, kick, ban, unban
//	    state, state/{eventType}, state/{eventType}/{stateKey}
//	    send/{eventType}/{txnId}
//	    messages, members
//	  sync
//	  profile/{userId} [/displayname | /avatar_url]
//	  user/{userId}/filter [/filterId]
//	  keys/upload, keys/query, keys/claim
//	  presence/{userId}/status
package client

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"

	"whale/room"
	"whale/user"
)

// =========================================================================
// Handler — shared dependencies + middleware
// =========================================================================

// Handler holds all dependencies for the C-S API handlers.
type Handler struct {
	DB            *gorm.DB
	UserSvc       *user.Service
	RoomSvc       *room.Service
	ServerName    string
	WellKnownCfg  WellKnownConfig
}

// WellKnownConfig controls /.well-known/matrix/* responses.
//
// These endpoints enable client and server discovery per the Matrix spec:
//
//	https://spec.matrix.org/latest/client-server-api/#getwell-knownmatrixclient
//	https://spec.matrix.org/latest/server-server-api/#getwell-knownmatrixserver
//
// When BaseURL is empty, the client well-known returns the server's own
// address. When FederationHost is empty, the server well-known returns
// the server_name with the federation port appended.
type WellKnownConfig struct {
	// BaseURL is the homeserver's base URL for Client-Server API.
	// Example: "https://matrix.example.com"
	// If empty, constructed from ServerName and the request's scheme.
	BaseURL string

	// FederationHost is the host:port for Server-Server federation.
	// Example: "matrix.example.com:8448"
	// If empty, constructed from ServerName and FederationPort.
	FederationHost string

	// FederationPort is the fallback port if FederationHost is empty.
	FederationPort int

	// IdentityServer is an optional identity server base URL for
	// the client well-known response.
	IdentityServer string
}

// NewHandler creates a Handler with all services wired up.
func NewHandler(db *gorm.DB, serverName string) *Handler {
	return &Handler{
		DB:         db,
		UserSvc:    &user.Service{DB: db, ServerName: serverName},
		RoomSvc:    &room.Service{DB: db, ServerName: serverName},
		ServerName: serverName,
	}
}

// RegisterRoutes mounts all Client-Server API routes.
func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// -- Unauthenticated --
	g.GET("/client/versions", h.Versions)
	g.GET("/client/v3/capabilities", h.Capabilities)

	// -- Auth endpoints --
	auth := g.Group("/client/v3")
	auth.POST("/login", h.Login)
	auth.POST("/register", h.Register)
	auth.POST("/logout", h.Logout, h.AuthRequired())
	auth.POST("/logout/all", h.LogoutAll, h.AuthRequired())
	auth.GET("/account/whoami", h.WhoAmI, h.AuthRequired())

	// -- Rooms --
	auth.POST("/createRoom", h.CreateRoom, h.AuthRequired())
	auth.GET("/joined_rooms", h.GetJoinedRooms, h.AuthRequired())
	auth.GET("/directory/room/:roomAlias", h.GetRoomAlias, h.AuthRequired())
	auth.PUT("/directory/room/:roomAlias", h.PutRoomAlias, h.AuthRequired())
	auth.DELETE("/directory/room/:roomAlias", h.DeleteRoomAlias, h.AuthRequired())

	rooms := auth.Group("/rooms/:roomId")
	rooms.POST("/join", h.JoinRoom, h.AuthRequired())
	rooms.POST("/leave", h.LeaveRoom, h.AuthRequired())
	rooms.POST("/invite", h.InviteUser, h.AuthRequired())
	rooms.POST("/kick", h.KickUser, h.AuthRequired())
	rooms.POST("/ban", h.BanUser, h.AuthRequired())
	rooms.POST("/unban", h.UnbanUser, h.AuthRequired())
	rooms.GET("/state", h.GetRoomState, h.AuthRequired())
	rooms.GET("/state/:eventType", h.GetRoomStateByType, h.AuthRequired())
	rooms.GET("/state/:eventType/:stateKey", h.GetRoomStateByTypeAndKey, h.AuthRequired())
	rooms.PUT("/send/:eventType/:txnId", h.SendEvent, h.AuthRequired())
	rooms.GET("/messages", h.GetMessages, h.AuthRequired())
	rooms.GET("/members", h.GetMembers, h.AuthRequired())

	// -- Sync --
	auth.GET("/sync", h.Sync, h.AuthRequired())

	// -- Profile --
	auth.GET("/profile/:userId", h.GetProfile, h.AuthRequired())
	auth.GET("/profile/:userId/displayname", h.GetDisplayName, h.AuthRequired())
	auth.PUT("/profile/:userId/displayname", h.SetDisplayName, h.AuthRequired())
	auth.GET("/profile/:userId/avatar_url", h.GetAvatarURL, h.AuthRequired())
	auth.PUT("/profile/:userId/avatar_url", h.SetAvatarURL, h.AuthRequired())

	// -- Filters --
	auth.POST("/user/:userId/filter", h.CreateFilter, h.AuthRequired())
	auth.GET("/user/:userId/filter/:filterId", h.GetFilter, h.AuthRequired())

	// -- E2EE Keys --
	auth.POST("/keys/upload", h.UploadKeys, h.AuthRequired())
	auth.POST("/keys/query", h.QueryKeys, h.AuthRequired())
	auth.POST("/keys/claim", h.ClaimKeys, h.AuthRequired())

	// -- Presence --
	auth.PUT("/presence/:userId/status", h.SetPresence, h.AuthRequired())
	auth.GET("/presence/:userId/status", h.GetPresence, h.AuthRequired())

	// -- Media --
	auth.POST("/media/upload", h.UploadMedia, h.AuthRequired())
}

// RegisterWellKnown mounts the /.well-known/matrix/* discovery endpoints
// directly on the root Echo instance (not under /_matrix). These endpoints
// must be served at the domain apex per RFC 8615 and the Matrix spec.
//
// Typical usage:
//
//	http://example.com/.well-known/matrix/client
//	http://example.com/.well-known/matrix/server
func (h *Handler) RegisterWellKnown(e *echo.Echo) {
	wk := e.Group("/.well-known/matrix")
	wk.GET("/client", h.WellKnownClient)
	wk.GET("/server", h.WellKnownServer)
	// CORS preflight — handled at the Echo level in main.go with:
	//   e.OPTIONS("/.well-known/matrix/*", h.ServeWellKnownOPTIONS)
}

// =========================================================================
// Auth middleware
// =========================================================================

// AuthRequired validates the access token and stores user_id in context.
func (h *Handler) AuthRequired() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := extractToken(c)
			if token == "" {
				return h.ErrorResponse(c, http.StatusUnauthorized, ErrMissingToken, "Missing access token")
			}
			userID, ok := h.UserSvc.ValidateToken(token)
			if !ok {
				return h.ErrorResponse(c, http.StatusUnauthorized, ErrUnknownToken, "Unknown or expired access token")
			}
			c.Set("user_id", userID)
			c.Set("access_token", token)
			return next(c)
		}
	}
}

// =========================================================================
// Standard Matrix error helpers
// =========================================================================

// MatrixError is the standard Matrix error response.
type MatrixError struct {
	ErrCode string `json:"errcode"`
	Error   string `json:"error"`
}

// ErrorResponse sends a standard Matrix error as JSON.
func (h *Handler) ErrorResponse(c echo.Context, status int, errCode, message string) error {
	return c.JSON(status, MatrixError{ErrCode: errCode, Error: message})
}

// Matrix error codes
const (
	ErrForbidden       = "M_FORBIDDEN"
	ErrUnknownToken    = "M_UNKNOWN_TOKEN"
	ErrMissingToken    = "M_MISSING_TOKEN"
	ErrBadJSON         = "M_BAD_JSON"
	ErrNotJSON         = "M_NOT_JSON"
	ErrNotFound        = "M_NOT_FOUND"
	ErrLimitExceeded   = "M_LIMIT_EXCEEDED"
	ErrInvalidParam    = "M_INVALID_PARAM"
	ErrUserInUse       = "M_USER_IN_USE"
	ErrInvalidUsername = "M_INVALID_USERNAME"
	ErrRoomInUse       = "M_ROOM_IN_USE"
	ErrUnknown         = "M_UNKNOWN"
	ErrTooLarge        = "M_TOO_LARGE"
	ErrUnsupported     = "M_UNSUPPORTED"
)

// =========================================================================
// Shared helpers
// =========================================================================

func extractToken(c echo.Context) string {
	if auth := c.Request().Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if t := c.QueryParam("access_token"); t != "" {
		return t
	}
	return ""
}

// GetUserID returns the authenticated user ID from context.
func GetUserID(c echo.Context) string {
	uid, _ := c.Get("user_id").(string)
	return uid
}

// Param is shorthand for c.Param.
func Param(c echo.Context, name string) string {
	return c.Param(name)
}

// BindJSON decodes the request body and writes a Matrix error on failure.
func BindJSON(c echo.Context, v interface{}) error {
	if err := json.NewDecoder(c.Request().Body).Decode(v); err != nil {
		return c.JSON(http.StatusBadRequest, MatrixError{ErrCode: ErrNotJSON, Error: "Invalid JSON: " + err.Error()})
	}
	return nil
}

// EventToDTO converts a storage.Event to an EventDTO for API responses.
func EventToDTO(ev EventLike) EventDTO {
	contentJSON, _ := json.Marshal(ev.EvtContent())
	unsignedJSON, _ := json.Marshal(ev.EvtUnsigned())
	return EventDTO{
		EventID:        ev.EvtEventID(),
		RoomID:         ev.EvtRoomID(),
		Sender:         ev.EvtSender(),
		EventType:      ev.EvtEventType(),
		StateKey:       ev.EvtStateKey(),
		Content:        contentJSON,
		OriginServerTS: ev.EvtOriginServerTS(),
		Unsigned:       unsignedJSON,
		Redacts:        ev.EvtRedacts(),
	}
}

// EventLike is the interface storage.Event satisfies for API conversion.
type EventLike interface {
	EvtEventID() string
	EvtRoomID() string
	EvtSender() string
	EvtEventType() string
	EvtStateKey() *string
	EvtContent() interface{}
	EvtOriginServerTS() int64
	EvtUnsigned() interface{}
	EvtRedacts() string
}

// queryInt parses a query parameter as int with a default.
func queryInt(c echo.Context, name string, defaultVal int) int {
	s := c.QueryParam(name)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}
