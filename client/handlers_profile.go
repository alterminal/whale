package client

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// =========================================================================
// GET /_matrix/client/v3/profile/{userId}
// =========================================================================

func (h *Handler) GetProfile(c echo.Context) error {
	userID := Param(c, "userId")

	// Normalize: if the client sent a bare localpart, prepend @ and append server name
	if !strings.HasPrefix(userID, "@") {
		userID = "@" + userID + ":" + h.ServerName
	}

	profile, err := h.UserSvc.GetProfile(userID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, ProfileResponse{
		DisplayName: profile.DisplayName,
		AvatarURL:   profile.AvatarURL,
	})
}

// =========================================================================
// GET /_matrix/client/v3/profile/{userId}/displayname
// =========================================================================

func (h *Handler) GetDisplayName(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	profile, err := h.UserSvc.GetProfile(userID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, err.Error())
	}

	if profile.DisplayName == "" {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "No display name set")
	}

	return c.JSON(http.StatusOK, DisplayNameRequest{DisplayName: profile.DisplayName})
}

// =========================================================================
// PUT /_matrix/client/v3/profile/{userId}/displayname
// =========================================================================

func (h *Handler) SetDisplayName(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	authUserID := GetUserID(c)

	// Only the user themselves can change their display name
	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot set another user's display name")
	}

	var req DisplayNameRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if err := h.UserSvc.SetDisplayName(userID, req.DisplayName); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// GET /_matrix/client/v3/profile/{userId}/avatar_url
// =========================================================================

func (h *Handler) GetAvatarURL(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	profile, err := h.UserSvc.GetProfile(userID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, err.Error())
	}

	if profile.AvatarURL == "" {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "No avatar URL set")
	}

	return c.JSON(http.StatusOK, AvatarURLRequest{AvatarURL: profile.AvatarURL})
}

// =========================================================================
// PUT /_matrix/client/v3/profile/{userId}/avatar_url
// =========================================================================

func (h *Handler) SetAvatarURL(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	authUserID := GetUserID(c)

	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot set another user's avatar")
	}

	var req AvatarURLRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if err := h.UserSvc.SetAvatarURL(userID, req.AvatarURL); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// Helper: normalizes a possibly-bare user ID to a full MXID
func normalizeUserID(userID, serverName string) string {
	if strings.HasPrefix(userID, "@") {
		return userID
	}
	if strings.Contains(userID, ":") {
		return userID
	}
	return "@" + userID + ":" + serverName
}
