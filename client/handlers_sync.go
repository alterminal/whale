package client

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// =========================================================================
// GET /_matrix/client/v3/sync
// =========================================================================

func (h *Handler) Sync(c echo.Context) error {
	userID := GetUserID(c)

	// Parse "since" as an integer stream position
	since := int64(0)
	if s := c.QueryParam("since"); s != "" {
		// since is an opaque token; for simplicity we encode it as a stream pos
		fmtInt64(c.QueryParam("since"), &since)
	}

	timeout := queryInt(c, "timeout", 0)

	// Build the sync response
	resp := SyncResponse{
		NextBatch:   "",
		Rooms:       &SyncRooms{},
	}

	// --- Joined rooms ---
	joinedRoomIDs, err := h.RoomSvc.GetJoinedRooms(userID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	joinMap := make(map[string]JoinedRoom)
	for _, roomID := range joinedRoomIDs {
		events, nextPos, err := h.RoomSvc.GetEventsSince(roomID, since, 50)
		if err != nil {
			continue
		}

		chunk := make([]EventDTO, 0, len(events))
		for _, ev := range events {
			chunk = append(chunk, EventToDTO(ev))
		}

		// First sync: include full state
		var stateEvents []EventDTO
		if since == 0 {
			state, _ := h.RoomSvc.GetState(roomID)
			for _, s := range state {
				stateEvents = append(stateEvents, EventDTO{
					EventID:   s.EventID,
					RoomID:    roomID,
					Sender:    "",
					EventType: s.EventType,
					StateKey:  &s.StateKey,
					Content:   mustMarshal(s.Content),
					OriginServerTS: time.Now().UnixMilli(),
				})
			}
		}

		joinMap[roomID] = JoinedRoom{
			State:    &StateSection{Events: stateEvents},
			Timeline: &TimelineSection{Events: chunk, Limited: false},
		}

		if nextPos > since {
			since = nextPos
		}
	}

	resp.Rooms.Join = joinMap

	// --- Invited rooms ---
	invited, err := h.RoomSvc.GetInvitedRooms(userID)
	if err == nil {
		inviteMap := make(map[string]InvitedRoom)
		for _, m := range invited {
			inviteMap[m.RoomID] = InvitedRoom{
				InviteState: &InviteStateSection{
					Events: []EventDTO{{
						EventID:   m.EventID,
						RoomID:    m.RoomID,
						Sender:    m.Sender,
						EventType: "m.room.member",
						StateKey:  sPtr(userID),
						Content:   mustMarshal(MemberContent{Membership: "invite", DisplayName: m.DisplayName}),
						OriginServerTS: m.CreatedAt.UnixMilli(),
					}},
				},
			}
		}
		resp.Rooms.Invite = inviteMap
	}

	// --- Next batch token ---
	if since == 0 {
		since = h.RoomSvc.MaxStreamPosition()
	}
	resp.NextBatch = fmtInt64Str(since)

	// Handle long-polling timeout
	if timeout > 0 && len(joinMap) == 0 && since > 0 {
		// Simple long-poll: wait briefly for new events
		select {
		case <-time.After(time.Duration(timeout) * time.Millisecond):
			if t := time.Duration(timeout) * time.Millisecond; t > 30*time.Second {
				_ = t
			}
		case <-c.Request().Context().Done():
			return nil
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// helpers

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func sPtr(s string) *string { return &s }

func fmtInt64(s string, out *int64) {
	// Minimal int64 parser
	var n int64
	var neg bool
	for _, ch := range s {
		if ch == '-' {
			neg = true
			continue
		}
		if ch >= '0' && ch <= '9' {
			n = n*10 + int64(ch-'0')
		}
	}
	if neg {
		n = -n
	}
	*out = n
}

func fmtInt64Str(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte(n%10) + '0'
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
