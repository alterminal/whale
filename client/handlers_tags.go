package client

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"whale/storage"
)

// =========================================================================
// Room Tags
// =========================================================================

// GET /_matrix/client/v3/user/{userId}/rooms/{roomId}/tags
func (h *Handler) GetRoomTags(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	roomID := Param(c, "roomId")
	authUserID := GetUserID(c)

	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot read another user's room tags")
	}

	var tags []storage.RoomTag
	if err := h.DB.Where("user_id = ? AND room_id = ?", userID, roomID).Find(&tags).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	result := TagsResponse{Tags: make(map[string]TagContent)}
	for _, t := range tags {
		orderJSON, _ := json.Marshal(t.Order)
		result.Tags[t.Tag] = TagContent{Order: orderJSON}
	}

	return c.JSON(http.StatusOK, result)
}

// PUT /_matrix/client/v3/user/{userId}/rooms/{roomId}/tags/{tag}
func (h *Handler) PutRoomTag(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	roomID := Param(c, "roomId")
	tag := Param(c, "tag")
	authUserID := GetUserID(c)

	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot set another user's room tags")
	}

	var req map[string]interface{}
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	order := storage.JSONMap{}
	if orderVal, ok := req["order"]; ok {
		if orderStr, ok := orderVal.(string); ok {
			order["order"] = orderStr
		} else {
			order["order"] = orderVal
		}
	}

	rt := storage.RoomTag{
		UserID: userID,
		RoomID: roomID,
		Tag:    tag,
		Order:  order,
	}

	if err := h.DB.Where("user_id = ? AND room_id = ? AND tag = ?", userID, roomID, tag).
		Assign(rt).FirstOrCreate(&rt).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// DELETE /_matrix/client/v3/user/{userId}/rooms/{roomId}/tags/{tag}
func (h *Handler) DeleteRoomTag(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	roomID := Param(c, "roomId")
	tag := Param(c, "tag")
	authUserID := GetUserID(c)

	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot delete another user's room tags")
	}

	if err := h.DB.Where("user_id = ? AND room_id = ? AND tag = ?", userID, roomID, tag).
		Delete(&storage.RoomTag{}).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}
