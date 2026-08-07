package client

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"whale/user"
)

// =========================================================================
// GET /_matrix/client/versions
// =========================================================================

func (h *Handler) Versions(c echo.Context) error {
	return c.JSON(http.StatusOK, VersionsResponse{
		Versions: []string{"r0.6.1", "v1.1", "v1.2", "v1.3", "v1.4", "v1.5", "v1.6"},
		UnstableFeatures: map[string]bool{
			"org.matrix.label_based_filtering":  true,
			"org.matrix.e2e_cross_signing":       true,
			"org.matrix.msc2285":                 true,
			"org.matrix.msc3916":                 true,
		},
	})
}

// =========================================================================
// GET /_matrix/client/v3/capabilities
// =========================================================================

func (h *Handler) Capabilities(c echo.Context) error {
	return c.JSON(http.StatusOK, CapabilitiesResponse{
		Capabilities: Capabilities{
			ChangePassword: &CapBool{Enabled: false},
			RoomVersions: &RoomVersionsCap{
				Default:   "10",
				Available: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"},
			},
		},
	})
}

// =========================================================================
// POST /_matrix/client/v3/login
// =========================================================================

func (h *Handler) Login(c echo.Context) error {
	var req LoginRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	var identifier *user.UserIdentifier
	if req.Identifier != nil {
		identifier = &user.UserIdentifier{
			Type: req.Identifier.Type,
			User: req.Identifier.User,
		}
	}

	result, err := h.UserSvc.Login(user.LoginParams{
		Type:          req.Type,
		User:          req.User,
		Identifier:    identifier,
		Password:      req.Password,
		Token:         req.Token,
		DeviceID:      req.DeviceID,
		InitialDisplayName: req.InitialDeviceDisplayName,
	})
	if err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, LoginResponse{
		UserID:      result.UserID,
		AccessToken: result.AccessToken,
		DeviceID:    result.DeviceID,
		HomeServer:  result.HomeServer,
	})
}

// =========================================================================
// POST /_matrix/client/v3/register
// =========================================================================

func (h *Handler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	// Phase 1: if auth dict is present, handle User-Interactive Auth
	if req.Auth != nil {
		// Simplified: for initial registration with type "m.login.dummy", just proceed.
		// Full UIA (e.g., email verification) is a future enhancement.
		if req.Auth.Type != "m.login.dummy" {
			return h.ErrorResponse(c, http.StatusUnauthorized, ErrForbidden, "Unsupported auth type")
		}
	}

	result, err := h.UserSvc.Register(user.RegisterParams{
		Username:             req.Username,
		Password:             req.Password,
		DeviceID:             req.DeviceID,
		InitialDisplayName:   req.InitialDeviceDisplayName,
		InhibitLogin:         req.InhibitLogin,
	})
	if err != nil {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidUsername, err.Error())
	}

	return c.JSON(http.StatusOK, RegisterResponse{
		UserID:      result.UserID,
		AccessToken: result.AccessToken,
		DeviceID:    result.DeviceID,
		HomeServer:  result.HomeServer,
	})
}

// =========================================================================
// POST /_matrix/client/v3/logout
// =========================================================================

func (h *Handler) Logout(c echo.Context) error {
	token, _ := c.Get("access_token").(string)
	if err := h.UserSvc.Logout(token); err != nil {
		return h.ErrorResponse(c, http.StatusUnauthorized, ErrUnknownToken, err.Error())
	}
	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/logout/all
// =========================================================================

func (h *Handler) LogoutAll(c echo.Context) error {
	userID := GetUserID(c)
	if err := h.UserSvc.LogoutAll(userID); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}
	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// GET /_matrix/client/v3/account/whoami
// =========================================================================

func (h *Handler) WhoAmI(c echo.Context) error {
	userID := GetUserID(c)
	return c.JSON(http.StatusOK, WhoAmIResponse{
		UserID:  userID,
		IsGuest: false,
	})
}
