package client

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"whale/room"
	"whale/storage"
)

// =========================================================================
// POST /_matrix/client/v3/createRoom
// =========================================================================

func (h *Handler) CreateRoom(c echo.Context) error {
	var req CreateRoomRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	result, err := h.RoomSvc.CreateRoom(room.CreateRoomParams{
		Creator:     GetUserID(c),
		Name:        req.Name,
		Topic:       req.Topic,
		Visibility:  req.Visibility,
		RoomAlias:   req.RoomAlias,
		Preset:      req.Preset,
		Invite:      req.Invite,
		IsDirect:    req.IsDirect,
		RoomVersion: req.RoomVersion,
	})
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, CreateRoomResponse{RoomID: result.RoomID})
}

// =========================================================================
// GET /_matrix/client/v3/joined_rooms
// =========================================================================

func (h *Handler) GetJoinedRooms(c echo.Context) error {
	userID := GetUserID(c)
	rooms, err := h.RoomSvc.GetJoinedRooms(userID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}
	if rooms == nil {
		rooms = []string{}
	}
	return c.JSON(http.StatusOK, JoinedRoomsResponse{JoinedRooms: rooms})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/join
// =========================================================================

func (h *Handler) JoinRoom(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	// Body is optional per spec — decode manually instead of using
	// BindJSON, which writes an error response on empty/malformed
	// bodies and causes a double-write when we continue below.
	var req JoinRequest
	if c.Request().Body != nil && c.Request().ContentLength > 0 {
		// Best-effort: ignore decode errors, body shape is not critical.
		json.NewDecoder(c.Request().Body).Decode(&req)
	}
	_ = req // reserved for future use (e.g., reason, third_party_signed)

	_, err := h.RoomSvc.JoinRoom(userID, roomID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, JoinResponse{RoomID: roomID})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/leave
// =========================================================================

func (h *Handler) LeaveRoom(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	if err := h.RoomSvc.LeaveRoom(userID, roomID); err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/invite
// =========================================================================

func (h *Handler) InviteUser(c echo.Context) error {
	roomID := Param(c, "roomId")
	senderID := GetUserID(c)

	var req InviteRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}
	if req.UserID == "" {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "user_id is required")
	}

	result, err := h.RoomSvc.InviteUser(roomID, senderID, req.UserID, req.Reason)
	if err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, InviteResponse{RoomID: result.RoomID})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/kick
// =========================================================================

func (h *Handler) KickUser(c echo.Context) error {
	roomID := Param(c, "roomId")
	senderID := GetUserID(c)

	var req KickRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}
	if req.UserID == "" {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "user_id is required")
	}

	if err := h.RoomSvc.KickUser(senderID, roomID, req.UserID, req.Reason); err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/ban
// =========================================================================

func (h *Handler) BanUser(c echo.Context) error {
	roomID := Param(c, "roomId")
	senderID := GetUserID(c)

	var req BanRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}
	if req.UserID == "" {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "user_id is required")
	}

	if err := h.RoomSvc.BanUser(senderID, roomID, req.UserID, req.Reason); err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/unban
// =========================================================================

func (h *Handler) UnbanUser(c echo.Context) error {
	roomID := Param(c, "roomId")
	senderID := GetUserID(c)

	var req UnbanRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}
	if req.UserID == "" {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "user_id is required")
	}

	if err := h.RoomSvc.UnbanUser(senderID, roomID, req.UserID); err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// GET /_matrix/client/v3/rooms/{roomId}/members
// =========================================================================

func (h *Handler) GetMembers(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	// Ensure the requester is a member
	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "You are not a member of this room")
	}

	memberships, err := h.RoomSvc.GetRoomMembers(roomID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	chunk := make([]MemberEventDTO, 0, len(memberships))
	for _, m := range memberships {
		chunk = append(chunk, MemberEventDTO{
			EventID:   m.EventID,
			Sender:    m.Sender,
			EventType: "m.room.member",
			StateKey:  m.UserID,
			Content: MemberContent{
				Membership:  m.Membership,
				DisplayName: m.DisplayName,
				AvatarURL:   m.AvatarURL,
				Reason:      m.Reason,
			},
			OriginServerTS: m.CreatedAt.UnixMilli(),
		})
	}

	return c.JSON(http.StatusOK, MembersResponse{Chunk: chunk})
}

// =========================================================================
// Room alias endpoints
// =========================================================================

func (h *Handler) GetRoomAlias(c echo.Context) error {
	alias := "#" + Param(c, "roomAlias") + ":" + h.Domain

	var ra storage.RoomAlias
	if err := h.DB.Where("alias = ?", alias).First(&ra).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Room alias not found")
		}
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, RoomAliasResponse{
		RoomID:  ra.RoomID,
		Servers: []string{h.Domain},
	})
}

func (h *Handler) PutRoomAlias(c echo.Context) error {
	alias := "#" + Param(c, "roomAlias") + ":" + h.Domain
	userID := GetUserID(c)

	var req RoomAliasRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}
	if req.RoomID == "" {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "room_id is required")
	}

	// Ensure the room exists and user is a member
	if !h.RoomSvc.IsMember(req.RoomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "You are not a member of this room")
	}

	// Verify the alias starts with our server name
	if !strings.HasSuffix(strings.TrimPrefix(alias, "#"), ":"+h.Domain) {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "Alias must end with :"+h.Domain)
	}

	ra := storage.RoomAlias{Alias: alias, RoomID: req.RoomID, Creator: userID}
	if err := h.DB.Where("alias = ?", alias).Assign(ra).FirstOrCreate(&ra).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

func (h *Handler) DeleteRoomAlias(c echo.Context) error {
	alias := "#" + Param(c, "roomAlias") + ":" + h.Domain
	userID := GetUserID(c)

	var ra storage.RoomAlias
	if err := h.DB.Where("alias = ?", alias).First(&ra).Error; err != nil {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Room alias not found")
	}

	// Allow deletion by the alias creator or any room member
	if ra.Creator != userID && !h.RoomSvc.IsMember(ra.RoomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not authorized to delete this alias")
	}

	h.DB.Delete(&ra)
	return c.JSON(http.StatusOK, EmptyResponse{})
}
