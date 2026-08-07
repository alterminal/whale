package client

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"whale/storage"
)

// =========================================================================
// GET /_matrix/client/v3/rooms/{roomId}/state
// =========================================================================

func (h *Handler) GetRoomState(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	state, err := h.RoomSvc.GetState(roomID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	events := make([]StateEventResponse, 0, len(state))
	for _, s := range state {
		contentJSON, _ := json.Marshal(s.Content)
		events = append(events, StateEventResponse{
			EventType: s.EventType,
			StateKey:  s.StateKey,
			Content:   contentJSON,
			EventID:   s.EventID,
		})
	}

	return c.JSON(http.StatusOK, events)
}

// =========================================================================
// GET /_matrix/client/v3/rooms/{roomId}/state/{eventType}
// =========================================================================

func (h *Handler) GetRoomStateByType(c echo.Context) error {
	roomID := Param(c, "roomId")
	eventType := Param(c, "eventType")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var states []storage.CurrentRoomState
	if err := h.DB.Where("room_id = ? AND event_type = ?", roomID, eventType).
		Find(&states).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	if len(states) == 0 {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "No state for this event type")
	}

	events := make([]StateEventResponse, len(states))
	for i, s := range states {
		contentJSON, _ := json.Marshal(s.Content)
		events[i] = StateEventResponse{
			EventType: s.EventType,
			StateKey:  s.StateKey,
			Content:   contentJSON,
			EventID:   s.EventID,
		}
	}

	return c.JSON(http.StatusOK, events)
}

// =========================================================================
// GET /_matrix/client/v3/rooms/{roomId}/state/{eventType}/{stateKey}
// =========================================================================

func (h *Handler) GetRoomStateByTypeAndKey(c echo.Context) error {
	roomID := Param(c, "roomId")
	eventType := Param(c, "eventType")
	stateKey := Param(c, "stateKey")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var s storage.CurrentRoomState
	if err := h.DB.Where("room_id = ? AND event_type = ? AND state_key = ?", roomID, eventType, stateKey).
		First(&s).Error; err != nil {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "State not found")
	}

	contentJSON, _ := json.Marshal(s.Content)
	return c.JSON(http.StatusOK, StateEventResponse{
		EventType: s.EventType,
		StateKey:  s.StateKey,
		Content:   contentJSON,
		EventID:   s.EventID,
	})
}

// =========================================================================
// PUT /_matrix/client/v3/rooms/{roomId}/send/{eventType}/{txnId}
// =========================================================================

func (h *Handler) SendEvent(c echo.Context) error {
	roomID := Param(c, "roomId")
	eventType := Param(c, "eventType")
	userID := GetUserID(c)

	// Read raw JSON body
	var raw json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&raw); err != nil {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrNotJSON, "Invalid JSON body")
	}

	switch eventType {
	case "m.room.message":
		// Parse as message event
		var content struct {
			MsgType       string `json:"msgtype"`
			Body          string `json:"body"`
			Format        string `json:"format,omitempty"`
			FormattedBody string `json:"formatted_body,omitempty"`
			URL           string `json:"url,omitempty"`
		}
		if err := json.Unmarshal(raw, &content); err != nil {
			return h.ErrorResponse(c, http.StatusBadRequest, ErrBadJSON, "Invalid message content")
		}
		if content.Body == "" {
			return h.ErrorResponse(c, http.StatusBadRequest, ErrBadJSON, "body is required")
		}

		eventID, err := h.RoomSvc.SendMessage(roomID, userID, content.MsgType, content.Body, content.FormattedBody, content.Format, Param(c, "txnId"))
		if err != nil {
			return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
		}
		return c.JSON(http.StatusOK, SendEventResponse{EventID: eventID})

	case "m.room.name", "m.room.topic", "m.room.join_rules", "m.room.guest_access",
		"m.room.history_visibility", "m.room.canonical_alias", "m.room.avatar":

		// State events — must be a joined member
		if !h.RoomSvc.IsMember(roomID, userID) {
			return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
		}

		// For now, just acknowledge. Full state event persistence with DAG
		// updating is non-trivial and deferred.
		return h.ErrorResponse(c, http.StatusNotImplemented, ErrUnsupported, "State event sending not yet fully implemented")

	default:
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "Unsupported event type: "+eventType)
	}
}

// =========================================================================
// GET /_matrix/client/v3/rooms/{roomId}/messages
// =========================================================================

func (h *Handler) GetMessages(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	from := c.QueryParam("from")
	dir := c.QueryParam("dir")
	if dir == "" {
		dir = "b"
	}
	limit := queryInt(c, "limit", 20)

	events, start, end, err := h.RoomSvc.GetMessages(roomID, from, dir, limit)
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	chunk := make([]EventDTO, 0, len(events))
	for _, ev := range events {
		chunk = append(chunk, EventToDTO(ev))
	}

	return c.JSON(http.StatusOK, MessagesResponse{
		Start: start,
		End:   end,
		Chunk: chunk,
	})
}
