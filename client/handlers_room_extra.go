package client

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"whale/storage"
)

// =========================================================================
// GET /_matrix/client/v3/rooms/{roomId}/event/{eventId}
// =========================================================================

func (h *Handler) GetEvent(c echo.Context) error {
	roomID := Param(c, "roomId")
	eventID := Param(c, "eventId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var ev storage.Event
	if err := h.DB.Where("room_id = ? AND event_id = ?", roomID, eventID).First(&ev).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Event not found")
		}
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EventToDTO(ev))
}

// =========================================================================
// GET /_matrix/client/v3/rooms/{roomId}/context/{eventId}
// =========================================================================

func (h *Handler) GetEventContext(c echo.Context) error {
	roomID := Param(c, "roomId")
	eventID := Param(c, "eventId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	limit := queryInt(c, "limit", 10)
	if limit > 100 {
		limit = 100
	}

	// Get the target event
	var targetEv storage.Event
	if err := h.DB.Where("room_id = ? AND event_id = ?", roomID, eventID).First(&targetEv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Event not found")
		}
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}
	targetDTO := EventToDTO(targetEv)

	// Events before
	var before []storage.Event
	h.DB.Where("room_id = ? AND origin_server_ts < ?", roomID, targetEv.OriginServerTS).
		Order("origin_server_ts DESC").Limit(limit).Find(&before)
	beforeDTOs := eventSliceToDTOs(reverseEvents(before))

	// Events after
	var after []storage.Event
	h.DB.Where("room_id = ? AND origin_server_ts > ?", roomID, targetEv.OriginServerTS).
		Order("origin_server_ts ASC").Limit(limit).Find(&after)
	afterDTOs := eventSliceToDTOs(after)

	// State at that point
	state, _ := h.RoomSvc.GetState(roomID)
	stateDTOs := make([]EventDTO, 0, len(state))
	for _, s := range state {
		contentJSON, _ := json.Marshal(s.Content)
		stateDTOs = append(stateDTOs, EventDTO{
			EventID:   s.EventID,
			RoomID:    roomID,
			EventType: s.EventType,
			StateKey:  &s.StateKey,
			Content:   contentJSON,
			OriginServerTS: time.Now().UnixMilli(),
		})
	}

	var start, end string
	if len(beforeDTOs) > 0 {
		start = beforeDTOs[0].EventID
	}
	if len(afterDTOs) > 0 {
		end = afterDTOs[len(afterDTOs)-1].EventID
	}

	return c.JSON(http.StatusOK, EventContextResponse{
		Start:        start,
		End:          end,
		EventsBefore: beforeDTOs,
		Event:        &targetDTO,
		EventsAfter:  afterDTOs,
		State:        stateDTOs,
	})
}

// =========================================================================
// PUT /_matrix/client/v3/rooms/{roomId}/redact/{eventId}/{txnId}
// =========================================================================

func (h *Handler) RedactEvent(c echo.Context) error {
	roomID := Param(c, "roomId")
	eventID := Param(c, "eventId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var req RedactRequest
	BindJSON(c, &req) // optional

	newEventID, err := h.RoomSvc.RedactEvent(userID, roomID, eventID, req.Reason)
	if err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, SendEventResponse{EventID: newEventID})
}

// =========================================================================
// PUT /_matrix/client/v3/rooms/{roomId}/typing/{userId}
// =========================================================================

func (h *Handler) SendTyping(c echo.Context) error {
	roomID := Param(c, "roomId")
	typingUserID := normalizeUserID(Param(c, "userId"), h.ServerName)
	userID := GetUserID(c)

	if userID != typingUserID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot set typing indicator for another user")
	}

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var req TypingRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	// Store typing state — ephemeral, just update the DB record
	typing := storage.Presence{} // reuse presence model, or could add a typing table
	_ = typing
	_ = req

	// For now, just acknowledge. Full typing notifications via /sync
	// require polling support which is deferred.
	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/receipt/{receiptType}/{eventId}
// =========================================================================

func (h *Handler) SendReceipt(c echo.Context) error {
	roomID := Param(c, "roomId")
	receiptType := Param(c, "receiptType")
	eventID := Param(c, "eventId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	// Parse optional thread_id from body
	var body struct {
		ThreadID string `json:"thread_id,omitempty"`
	}
	BindJSON(c, &body) // optional

	receipt := storage.Receipt{
		UserID:      userID,
		RoomID:      roomID,
		ReceiptType: receiptType,
		EventID:     eventID,
		ThreadID:    body.ThreadID,
	}

	if err := h.DB.Where("user_id = ? AND room_id = ? AND receipt_type = ?",
		userID, roomID, receiptType).
		Assign(receipt).FirstOrCreate(&receipt).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// GET /_matrix/client/v3/rooms/{roomId}/joined_members
// =========================================================================

func (h *Handler) GetJoinedMembers(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	memberships, err := h.RoomSvc.GetRoomMembers(roomID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	joined := make(map[string]JoinedMemberInfo)
	for _, m := range memberships {
		if m.Membership == "join" {
			joined[m.UserID] = JoinedMemberInfo{
				DisplayName: m.DisplayName,
				AvatarURL:   m.AvatarURL,
			}
		}
	}

	return c.JSON(http.StatusOK, JoinedMembersResponse{Joined: joined})
}

// =========================================================================
// PUT /_matrix/client/v3/rooms/{roomId}/state/{eventType}/{stateKey}
// =========================================================================

func (h *Handler) SetRoomState(c echo.Context) error {
	roomID := Param(c, "roomId")
	eventType := Param(c, "eventType")
	stateKey := Param(c, "stateKey")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var raw json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&raw); err != nil {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrNotJSON, "Invalid JSON body")
	}

	eventID, err := h.RoomSvc.SetStateEvent(roomID, userID, eventType, stateKey, raw)
	if err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, SendEventResponse{EventID: eventID})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/report/{eventId}
// =========================================================================

func (h *Handler) ReportEvent(c echo.Context) error {
	roomID := Param(c, "roomId")
	eventID := Param(c, "eventId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var req ReportRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	// For now, just log the report. Full abuse reporting infrastructure deferred.
	_ = eventID
	_ = req

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/forget
// =========================================================================

func (h *Handler) ForgetRoom(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	if err := h.RoomSvc.ForgetRoom(userID, roomID); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/rooms/{roomId}/read_markers
// =========================================================================

func (h *Handler) SetReadMarkers(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var req ReadMarkersRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	// Store fully_read as room account data
	if req.FullyRead != "" {
		ad := storage.RoomAccountData{
			UserID:   userID,
			RoomID:   roomID,
			DataType: "m.fully_read",
			Content:  storage.JSONMap{"event_id": req.FullyRead},
		}
		h.DB.Where("user_id = ? AND room_id = ? AND data_type = ?", userID, roomID, "m.fully_read").
			Assign(ad).FirstOrCreate(&ad)
	}

	// m.read and m.read.private are stored as receipts
	if req.Read != "" {
		receipt := storage.Receipt{
			UserID:      userID,
			RoomID:      roomID,
			ReceiptType: "m.read",
			EventID:     req.Read,
		}
		h.DB.Where("user_id = ? AND room_id = ? AND receipt_type = ?", userID, roomID, "m.read").
			Assign(receipt).FirstOrCreate(&receipt)
	}

	if req.ReadPrivate != "" {
		receipt := storage.Receipt{
			UserID:      userID,
			RoomID:      roomID,
			ReceiptType: "m.read.private",
			EventID:     req.ReadPrivate,
		}
		h.DB.Where("user_id = ? AND room_id = ? AND receipt_type = ?", userID, roomID, "m.read.private").
			Assign(receipt).FirstOrCreate(&receipt)
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// Helpers
// =========================================================================

func eventSliceToDTOs(events []storage.Event) []EventDTO {
	dtos := make([]EventDTO, 0, len(events))
	for _, ev := range events {
		dtos = append(dtos, EventToDTO(ev))
	}
	return dtos
}

func reverseEvents(events []storage.Event) []storage.Event {
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events
}
