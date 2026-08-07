package client

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"whale/storage"
)

// =========================================================================
// Public Rooms
// =========================================================================

// GET /_matrix/client/v3/publicRooms
func (h *Handler) GetPublicRooms(c echo.Context) error {
	limit := queryInt(c, "limit", 50)
	if limit > 100 {
		limit = 100
	}
	since := c.QueryParam("since")

	var rooms []storage.Room
	tx := h.DB.Where("is_public = ?", true)
	if since != "" {
		tx = tx.Where("room_id > ?", since)
	}
	tx = tx.Order("room_id ASC").Limit(limit + 1)

	if err := tx.Find(&rooms).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	chunk := make([]PublicRoomDTO, 0, len(rooms))
	for i, room := range rooms {
		if i >= limit {
			break
		}

		// Count joined members
		var memberCount int64
		h.DB.Model(&storage.RoomMembership{}).
			Where("room_id = ? AND membership = ?", room.RoomID, "join").
			Count(&memberCount)

		// Get aliases
		var aliases []storage.RoomAlias
		h.DB.Where("room_id = ?", room.RoomID).Find(&aliases)
		aliasStrs := make([]string, len(aliases))
		for j, a := range aliases {
			aliasStrs[j] = a.Alias
		}

		chunk = append(chunk, PublicRoomDTO{
			RoomID:           room.RoomID,
			Name:             room.Name,
			Topic:            room.Topic,
			CanonicalAlias:   room.CanonicalAlias,
			Aliases:          aliasStrs,
			NumJoinedMembers: int(memberCount),
			WorldReadable:    room.IsPublic,
			GuestCanJoin:     room.JoinRules == "public",
			JoinRule:         room.JoinRules,
		})
	}

	resp := PublicRoomsResponse{Chunk: chunk}
	if len(rooms) > limit {
		resp.NextBatch = rooms[limit].RoomID
	}

	return c.JSON(http.StatusOK, resp)
}

// POST /_matrix/client/v3/publicRooms (filtered search)
func (h *Handler) PostPublicRooms(c echo.Context) error {
	var req PublicRoomsRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var rooms []storage.Room
	tx := h.DB.Where("is_public = ?", true)

	if req.Filter != nil && req.Filter.GenericSearchTerm != "" {
		term := "%" + req.Filter.GenericSearchTerm + "%"
		tx = tx.Where("name LIKE ? OR topic LIKE ? OR canonical_alias LIKE ?", term, term, term)
	}

	if req.Since != "" {
		tx = tx.Where("room_id > ?", req.Since)
	}

	tx = tx.Order("room_id ASC").Limit(limit + 1)

	if err := tx.Find(&rooms).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	chunk := make([]PublicRoomDTO, 0, len(rooms))
	for i, room := range rooms {
		if i >= limit {
			break
		}

		var memberCount int64
		h.DB.Model(&storage.RoomMembership{}).
			Where("room_id = ? AND membership = ?", room.RoomID, "join").
			Count(&memberCount)

		var aliases []storage.RoomAlias
		h.DB.Where("room_id = ?", room.RoomID).Find(&aliases)
		aliasStrs := make([]string, len(aliases))
		for j, a := range aliases {
			aliasStrs[j] = a.Alias
		}

		chunk = append(chunk, PublicRoomDTO{
			RoomID:           room.RoomID,
			Name:             room.Name,
			Topic:            room.Topic,
			CanonicalAlias:   room.CanonicalAlias,
			Aliases:          aliasStrs,
			NumJoinedMembers: int(memberCount),
			WorldReadable:    room.IsPublic,
			GuestCanJoin:     room.JoinRules == "public",
			JoinRule:         room.JoinRules,
		})
	}

	resp := PublicRoomsResponse{Chunk: chunk}
	if len(rooms) > limit {
		resp.NextBatch = rooms[limit].RoomID
	}

	return c.JSON(http.StatusOK, resp)
}

// =========================================================================
// Directory visibility
// =========================================================================

// PUT /_matrix/client/v3/directory/list/room/{roomId}
func (h *Handler) SetRoomVisibility(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var req RoomVisibilityRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if req.Visibility != "public" && req.Visibility != "private" {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "visibility must be 'public' or 'private'")
	}

	isPublic := req.Visibility == "public"
	if err := h.DB.Model(&storage.Room{}).Where("room_id = ?", roomID).
		Update("is_public", isPublic).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// GET /_matrix/client/v3/directory/list/room/{roomId}
func (h *Handler) GetRoomVisibility(c echo.Context) error {
	roomID := Param(c, "roomId")

	var room storage.Room
	if err := h.DB.Where("room_id = ?", roomID).First(&room).Error; err != nil {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Room not found")
	}

	visibility := "private"
	if room.IsPublic {
		visibility = "public"
	}

	return c.JSON(http.StatusOK, RoomVisibilityResponse{Visibility: visibility})
}
