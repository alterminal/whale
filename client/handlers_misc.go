package client

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"whale/storage"
)

// =========================================================================
// PUT /_matrix/client/v3/presence/{userId}/status
// =========================================================================

func (h *Handler) SetPresence(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	authUserID := GetUserID(c)

	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot set another user's presence")
	}

	var req PresenceRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if req.Presence != "online" && req.Presence != "offline" && req.Presence != "unavailable" {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "presence must be one of: online, offline, unavailable")
	}

	presence := storage.Presence{
		UserID:          userID,
		Presence:        req.Presence,
		StatusMessage:   req.StatusMessage,
		CurrentlyActive: req.Presence == "online",
		LastActiveAt:    time.Now().UnixMilli(),
	}

	if err := h.DB.Where("user_id = ?", userID).
		Assign(presence).FirstOrCreate(&presence).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// GET /_matrix/client/v3/presence/{userId}/status
// =========================================================================

func (h *Handler) GetPresence(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)

	var presence storage.Presence
	if err := h.DB.Where("user_id = ?", userID).First(&presence).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Default: offline
			return c.JSON(http.StatusOK, PresenceStatusResponse{
				Presence:        "offline",
				CurrentlyActive: false,
				LastActiveAgo:   0,
			})
		}
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	lastActiveAgo := time.Now().UnixMilli() - presence.LastActiveAt

	return c.JSON(http.StatusOK, PresenceStatusResponse{
		Presence:        presence.Presence,
		StatusMessage:   presence.StatusMessage,
		CurrentlyActive: presence.CurrentlyActive,
		LastActiveAgo:   lastActiveAgo,
	})
}

// =========================================================================
// POST /_matrix/client/v3/user/{userId}/filter
// =========================================================================

func (h *Handler) CreateFilter(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	authUserID := GetUserID(c)

	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot create filter for another user")
	}

	var req FilterRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	filterID := generateFilterID()
	f := storage.Filter{
		UserID:    userID,
		FilterID:  filterID,
		FilterDef: storage.JSONMap{
			"account_data": req.AccountData,
			"presence":     req.Presence,
			"room":         req.Room,
		},
	}

	if err := h.DB.Create(&f).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, FilterResponse{FilterID: filterID})
}

// =========================================================================
// GET /_matrix/client/v3/user/{userId}/filter/{filterId}
// =========================================================================

func (h *Handler) GetFilter(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	filterID := Param(c, "filterId")
	authUserID := GetUserID(c)

	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot access another user's filter")
	}

	var f storage.Filter
	if err := h.DB.Where("user_id = ? AND filter_id = ?", userID, filterID).First(&f).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Filter not found")
		}
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	// Return the stored filter definition
	return c.JSON(http.StatusOK, f.FilterDef)
}

// =========================================================================
// POST /_matrix/client/v3/media/upload
// =========================================================================

func (h *Handler) UploadMedia(c echo.Context) error {
	// Read form file
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "No file provided")
	}

	// Limit to 50MB
	if file.Size > 50*1024*1024 {
		return h.ErrorResponse(c, http.StatusRequestEntityTooLarge, ErrTooLarge, "File too large (max 50MB)")
	}

	// Generate media ID
	mediaID := generateFilterID() // reuse random hex generator

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Save to disk (for now, just store the metadata — actual file storage TODO)
	// In production, save to a media directory or object store.
	mediaRecord := storage.MediaRecord{
		MediaID:     mediaID,
		Origin:      h.ServerName,
		ContentType: contentType,
		SizeBytes:   file.Size,
		Uploader:    GetUserID(c),
		FilePath:    "media/" + mediaID,
	}

	if err := h.DB.Create(&mediaRecord).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	contentURI := "mxc://" + h.ServerName + "/" + mediaID
	return c.JSON(http.StatusOK, MediaUploadResponse{ContentURI: contentURI})
}
