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
	Domain        string // MXID domain part (e.g., "alterminal.com")
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
func NewHandler(db *gorm.DB, serverName, domain string) *Handler {
	return &Handler{
		DB:         db,
		UserSvc:    &user.Service{DB: db, ServerName: serverName, Domain: domain},
		RoomSvc:    &room.Service{DB: db, ServerName: serverName, Domain: domain},
		ServerName: serverName,
		Domain:     domain,
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
	auth.GET("/login", h.GetLoginFlows)
	auth.POST("/login", h.Login)
	auth.POST("/register", h.Register)
	auth.POST("/logout", h.Logout, h.AuthRequired())
	auth.POST("/logout/all", h.LogoutAll, h.AuthRequired())
	auth.POST("/refresh", h.RefreshToken) // unauthenticated (uses refresh token)
	auth.GET("/account/whoami", h.WhoAmI, h.AuthRequired())
	auth.POST("/account/password", h.ChangePassword, h.AuthRequired())
	auth.POST("/account/deactivate", h.DeactivateAccount, h.AuthRequired())

	// -- Rooms --
	auth.POST("/createRoom", h.CreateRoom, h.AuthRequired())
	auth.GET("/joined_rooms", h.GetJoinedRooms, h.AuthRequired())
	auth.GET("/directory/room/:roomAlias", h.GetRoomAlias, h.AuthRequired())
	auth.PUT("/directory/room/:roomAlias", h.PutRoomAlias, h.AuthRequired())
	auth.DELETE("/directory/room/:roomAlias", h.DeleteRoomAlias, h.AuthRequired())

	// Directory list (visibility)
	auth.PUT("/directory/list/room/:roomId", h.SetRoomVisibility, h.AuthRequired())
	auth.GET("/directory/list/room/:roomId", h.GetRoomVisibility, h.AuthRequired())

	rooms := auth.Group("/rooms/:roomId")
	rooms.POST("/join", h.JoinRoom, h.AuthRequired())
	rooms.POST("/leave", h.LeaveRoom, h.AuthRequired())
	rooms.POST("/invite", h.InviteUser, h.AuthRequired())
	rooms.POST("/kick", h.KickUser, h.AuthRequired())
	rooms.POST("/ban", h.BanUser, h.AuthRequired())
	rooms.POST("/unban", h.UnbanUser, h.AuthRequired())
	rooms.POST("/forget", h.ForgetRoom, h.AuthRequired())
	rooms.POST("/upgrade", h.UpgradeRoom, h.AuthRequired())
	rooms.POST("/report/:eventId", h.ReportEvent, h.AuthRequired())
	rooms.GET("/state", h.GetRoomState, h.AuthRequired())
	rooms.GET("/state/:eventType", h.GetRoomStateByType, h.AuthRequired())
	rooms.GET("/state/:eventType/:stateKey", h.GetRoomStateByTypeAndKey, h.AuthRequired())
	rooms.PUT("/state/:eventType/:stateKey", h.SetRoomState, h.AuthRequired())
	rooms.PUT("/send/:eventType/:txnId", h.SendEvent, h.AuthRequired())
	rooms.PUT("/redact/:eventId/:txnId", h.RedactEvent, h.AuthRequired())
	rooms.GET("/messages", h.GetMessages, h.AuthRequired())
	rooms.GET("/members", h.GetMembers, h.AuthRequired())
	rooms.GET("/joined_members", h.GetJoinedMembers, h.AuthRequired())
	rooms.GET("/event/:eventId", h.GetEvent, h.AuthRequired())
	rooms.GET("/context/:eventId", h.GetEventContext, h.AuthRequired())
	rooms.PUT("/typing/:userId", h.SendTyping, h.AuthRequired())
	rooms.POST("/receipt/:receiptType/:eventId", h.SendReceipt, h.AuthRequired())
	rooms.POST("/read_markers", h.SetReadMarkers, h.AuthRequired())

	// -- Relations / Threads (stub for now) --
	rooms.GET("/relations/:eventId", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{"chunk": []interface{}{}})
	}, h.AuthRequired())
	rooms.GET("/relations/:eventId/:relType", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{"chunk": []interface{}{}})
	}, h.AuthRequired())
	rooms.GET("/threads", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{"chunk": []interface{}{}})
	}, h.AuthRequired())

	// -- Sync --
	auth.GET("/sync", h.Sync, h.AuthRequired())

	// -- Profile --
	auth.GET("/profile/:userId", h.GetProfile, h.AuthRequired())
	auth.GET("/profile/:userId/displayname", h.GetDisplayName, h.AuthRequired())
	auth.PUT("/profile/:userId/displayname", h.SetDisplayName, h.AuthRequired())
	auth.GET("/profile/:userId/avatar_url", h.GetAvatarURL, h.AuthRequired())
	auth.PUT("/profile/:userId/avatar_url", h.SetAvatarURL, h.AuthRequired())

	// -- Account Data --
	auth.GET("/user/:userId/account_data/:type", h.GetAccountData, h.AuthRequired())
	auth.PUT("/user/:userId/account_data/:type", h.PutAccountData, h.AuthRequired())
	auth.GET("/user/:userId/rooms/:roomId/account_data/:type", h.GetRoomAccountData, h.AuthRequired())
	auth.PUT("/user/:userId/rooms/:roomId/account_data/:type", h.PutRoomAccountData, h.AuthRequired())

	// -- Filters --
	auth.POST("/user/:userId/filter", h.CreateFilter, h.AuthRequired())
	auth.GET("/user/:userId/filter/:filterId", h.GetFilter, h.AuthRequired())

	// -- Room Tags --
	auth.GET("/user/:userId/rooms/:roomId/tags", h.GetRoomTags, h.AuthRequired())
	auth.PUT("/user/:userId/rooms/:roomId/tags/:tag", h.PutRoomTag, h.AuthRequired())
	auth.DELETE("/user/:userId/rooms/:roomId/tags/:tag", h.DeleteRoomTag, h.AuthRequired())

	// -- E2EE Keys --
	auth.POST("/keys/upload", h.UploadKeys, h.AuthRequired())
	auth.POST("/keys/query", h.QueryKeys, h.AuthRequired())
	auth.POST("/keys/claim", h.ClaimKeys, h.AuthRequired())

	// -- Presence --
	auth.PUT("/presence/:userId/status", h.SetPresence, h.AuthRequired())
	auth.GET("/presence/:userId/status", h.GetPresence, h.AuthRequired())

	// -- Media --
	auth.POST("/media/upload", h.UploadMediaEnhanced, h.AuthRequired())
	g.GET("/media/download/:serverName/:mediaId", h.DownloadMedia) // may be accessed without auth
	g.GET("/media/thumbnail/:serverName/:mediaId", h.ThumbnailMedia)
	auth.GET("/media/config", h.MediaConfig)
	auth.GET("/media/preview_url", h.PreviewURL, h.AuthRequired())

	// -- Devices --
	auth.GET("/devices", h.GetDevices, h.AuthRequired())
	auth.GET("/devices/:deviceId", h.GetDevice, h.AuthRequired())
	auth.PUT("/devices/:deviceId", h.UpdateDevice, h.AuthRequired())
	auth.DELETE("/devices/:deviceId", h.DeleteDevice, h.AuthRequired())
	auth.POST("/delete_devices", h.DeleteDevices, h.AuthRequired())

	// -- Search --
	auth.POST("/search", h.Search, h.AuthRequired())

	// -- Public Rooms --
	auth.GET("/publicRooms", h.GetPublicRooms)
	auth.POST("/publicRooms", h.PostPublicRooms, h.AuthRequired())

	// -- Knock --
	auth.POST("/knock/:roomIdOrAlias", h.KnockRoom, h.AuthRequired())

	// -- VoIP --
	auth.GET("/voip/turnServer", h.TurnServer, h.AuthRequired())

	// -- OpenID --
	auth.POST("/user/:userId/openid/request_token", h.OpenIDToken, h.AuthRequired())

	// -- Push Notifications --
	auth.POST("/pushers/set", h.SetPusher, h.AuthRequired())
	auth.GET("/pushers", h.GetPushers, h.AuthRequired())
	auth.GET("/notifications", h.GetNotifications, h.AuthRequired())

	// -- Push Rules --
	auth.GET("/pushrules/", h.GetPushRules, h.AuthRequired())
	auth.GET("/pushrules/:scope/:kind/:ruleId", h.GetPushRule, h.AuthRequired())
	auth.PUT("/pushrules/:scope/:kind/:ruleId", h.PutPushRule, h.AuthRequired())
	auth.DELETE("/pushrules/:scope/:kind/:ruleId", h.DeletePushRule, h.AuthRequired())
	auth.PUT("/pushrules/:scope/:kind/:ruleId/enabled", h.SetPushRuleEnabled, h.AuthRequired())
	auth.PUT("/pushrules/:scope/:kind/:ruleId/actions", h.SetPushRuleActions, h.AuthRequired())

	// -- Third-party --
	auth.GET("/thirdparty/protocols", h.GetThirdPartyProtocols, h.AuthRequired())
	auth.GET("/thirdparty/protocol/:protocol", h.GetThirdPartyProtocol, h.AuthRequired())
	auth.GET("/thirdparty/location", h.GetThirdPartyLocation, h.AuthRequired())
	auth.GET("/thirdparty/user", h.GetThirdPartyUser, h.AuthRequired())
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
