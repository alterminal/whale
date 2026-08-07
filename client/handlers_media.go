package client

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"whale/storage"
)

// =========================================================================
// Media download / thumbnail / config / preview
// =========================================================================

// GET /_matrix/client/v3/media/download/{serverName}/{mediaId}
func (h *Handler) DownloadMedia(c echo.Context) error {
	serverName := Param(c, "serverName")
	mediaID := Param(c, "mediaId")

	var media storage.MediaRecord
	if err := h.DB.Where("origin = ? AND media_id = ?", serverName, mediaID).First(&media).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Media not found")
		}
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	// Check if file exists on disk
	filePath := media.FilePath
	if filePath == "" {
		filePath = filepath.Join("media", mediaID)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File not on disk — attempt alternative path or return placeholder
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Media file not found on disk")
	}

	// Allow cross-origin for media
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Content-Type", media.ContentType)
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", mediaID))
	c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	return c.File(filePath)
}

// GET /_matrix/client/v3/media/thumbnail/{serverName}/{mediaId}
func (h *Handler) ThumbnailMedia(c echo.Context) error {
	serverName := Param(c, "serverName")
	mediaID := Param(c, "mediaId")

	var media storage.MediaRecord
	if err := h.DB.Where("origin = ? AND media_id = ?", serverName, mediaID).First(&media).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Media not found")
		}
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	// Thumbnail generation is deferred — return original for now
	filePath := media.FilePath
	if filePath == "" {
		filePath = filepath.Join("media", mediaID)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Media file not found on disk")
	}

	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Content-Type", media.ContentType)
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")

	return c.File(filePath)
}

// GET /_matrix/client/v3/media/config
func (h *Handler) MediaConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"m.upload.size": 52428800, // 50 MB
	})
}

// GET /_matrix/client/v3/media/preview_url
func (h *Handler) PreviewURL(c echo.Context) error {
	// URL preview is complex — defer
	return h.ErrorResponse(c, http.StatusNotImplemented, ErrUnsupported, "URL preview not yet implemented")
}

// =========================================================================
// Improved media upload — also saves file to disk
// =========================================================================

// POST /_matrix/client/v3/media/upload (enhanced version)
func (h *Handler) UploadMediaEnhanced(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "No file provided")
	}

	if file.Size > 50*1024*1024 {
		return h.ErrorResponse(c, http.StatusRequestEntityTooLarge, ErrTooLarge, "File too large (max 50MB)")
	}

	mediaID := generateFilterID()
	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Ensure media directory exists
	mediaDir := "media"
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, "Failed to create media directory")
	}

	filePath := filepath.Join(mediaDir, mediaID)

	// Read uploaded file
	src, err := file.Open()
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, "Failed to read uploaded file")
	}
	defer src.Close()

	dst, err := os.Create(filePath)
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, "Failed to save file")
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, "Failed to write file")
	}

	mediaRecord := storage.MediaRecord{
		MediaID:     mediaID,
		Origin:      h.ServerName,
		ContentType: contentType,
		SizeBytes:   file.Size,
		Uploader:    GetUserID(c),
		FilePath:    filePath,
	}

	if err := h.DB.Create(&mediaRecord).Error; err != nil {
		os.Remove(filePath) // clean up
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	contentURI := "mxc://" + h.ServerName + "/" + mediaID
	return c.JSON(http.StatusOK, MediaUploadResponse{ContentURI: contentURI})
}
