// Package room provides room creation, membership management, event
// construction, and state resolution.
package room

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"whale/storage"
)

// Service wraps the database and server configuration for room operations.
type Service struct {
	DB         *gorm.DB
	ServerName string
	Domain     string // MXID domain part for room IDs, event IDs, and aliases
}

// CreateRoomParams holds the request to create a new room.
type CreateRoomParams struct {
	Creator     string `json:"-"` // set by the handler from auth
	Name        string `json:"name,omitempty"`
	Topic       string `json:"topic,omitempty"`
	Visibility  string `json:"visibility,omitempty"` // "public" or "private"
	RoomAlias   string `json:"room_alias_name,omitempty"`
	Preset      string `json:"preset,omitempty"` // "private_chat", "trusted_private_chat", "public_chat"
	Invite      []string `json:"invite,omitempty"`
	IsDirect    bool     `json:"is_direct,omitempty"`
	RoomVersion string   `json:"room_version,omitempty"`
}

// CreateRoomResult is the response for POST /createRoom.
type CreateRoomResult struct {
	RoomID string `json:"room_id"`
}

// InviteResult is returned from invite operations.
type InviteResult struct {
	RoomID        string `json:"room_id"`
	InvitedUserID string `json:"user_id"`
}

// ---------------------------------------------------------------------------
// Room lifecycle
// ---------------------------------------------------------------------------

// CreateRoom creates a new room with the creator as the first joined member.
//
// It generates a unique room_id, inserts the room record, fires the initial
// state events (m.room.create, m.room.member, m.room.power_levels,
// m.room.join_rules, m.room.name, m.room.topic), and sets up the creator's
// membership as "join".
func (s *Service) CreateRoom(params CreateRoomParams) (*CreateRoomResult, error) {
	if params.Creator == "" {
		return nil, errors.New("creator is required")
	}

	roomID := generateRoomID(s.Domain)
	now := time.Now()
	originTS := now.UnixMilli()

	version := params.RoomVersion
	if version == "" {
		version = "10"
	}

	// Determine join rules from preset
	joinRules := "invite"
	isPublic := false
	switch params.Preset {
	case "public_chat":
		joinRules = "public"
		isPublic = true
	case "private_chat", "trusted_private_chat":
		joinRules = "invite"
	}

	// Override with explicit visibility
	if params.Visibility == "public" {
		joinRules = "public"
		isPublic = true
	} else if params.Visibility == "private" {
		joinRules = "invite"
	}

	// Run everything in a transaction
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Create room record
		room := storage.Room{
			RoomID:      roomID,
			Name:        params.Name,
			Topic:       params.Topic,
			Creator:     params.Creator,
			RoomVersion: version,
			IsPublic:    isPublic,
			JoinRules:   joinRules,
		}
		if err := tx.Create(&room).Error; err != nil {
			return fmt.Errorf("create room: %w", err)
		}

		// 2. m.room.create event (depth 1)
		createContent := storage.EventContent{
			Raw: storage.JSONMap{"creator": params.Creator, "room_version": version},
		}
		createEvent := s.newEvent(roomID, params.Creator, "m.room.create", "", createContent, originTS, 1, nil, nil)
		if err := tx.Create(&createEvent).Error; err != nil {
			return fmt.Errorf("create m.room.create: %w", err)
		}

		// 3. m.room.member for creator (join) — depth 2, prev = create event
		memberContent := storage.EventContent{
			Membership:  "join",
			DisplayName: params.Creator,
			Raw:         storage.JSONMap{"membership": "join"},
		}
		memberEvent := s.newEvent(roomID, params.Creator, "m.room.member", params.Creator, memberContent, originTS, 2, []string{createEvent.EventID}, nil)
		if err := tx.Create(&memberEvent).Error; err != nil {
			return fmt.Errorf("create m.room.member: %w", err)
		}

		// 4. m.room.power_levels — depth 3
		plContent := storage.EventContent{
			Raw: defaultPowerLevels(params.Creator),
		}
		plEvent := s.newEvent(roomID, params.Creator, "m.room.power_levels", "", plContent, originTS, 3, []string{memberEvent.EventID}, nil)
		if err := tx.Create(&plEvent).Error; err != nil {
			return fmt.Errorf("create m.room.power_levels: %w", err)
		}

		// 5. m.room.join_rules — depth 4
		jrContent := storage.EventContent{
			JoinRule: joinRules,
			Raw:      storage.JSONMap{"join_rule": joinRules},
		}
		jrEvent := s.newEvent(roomID, params.Creator, "m.room.join_rules", "", jrContent, originTS, 4, []string{plEvent.EventID}, nil)
		if err := tx.Create(&jrEvent).Error; err != nil {
			return fmt.Errorf("create m.room.join_rules: %w", err)
		}

		// Track the frontier for subsequent events
		frontier := []string{jrEvent.EventID}
		depth := int64(5)

		// 6. m.room.name (if provided)
		if params.Name != "" {
			nameEvent := s.newEvent(roomID, params.Creator, "m.room.name", "", storage.EventContent{
				Name: params.Name,
				Raw:  storage.JSONMap{"name": params.Name},
			}, originTS, depth, frontier, nil)
			if err := tx.Create(&nameEvent).Error; err != nil {
				return fmt.Errorf("create m.room.name: %w", err)
			}
			frontier = []string{nameEvent.EventID}
			depth++
		}

		// 7. m.room.topic (if provided)
		if params.Topic != "" {
			topicEvent := s.newEvent(roomID, params.Creator, "m.room.topic", "", storage.EventContent{
				Topic: params.Topic,
				Raw:   storage.JSONMap{"topic": params.Topic},
			}, originTS, depth, frontier, nil)
			if err := tx.Create(&topicEvent).Error; err != nil {
				return fmt.Errorf("create m.room.topic: %w", err)
			}
			frontier = []string{topicEvent.EventID}
			depth++
		}

		// 8. Creator membership record
		membership := storage.RoomMembership{
			RoomID:     roomID,
			UserID:     params.Creator,
			Sender:     params.Creator,
			Membership: "join",
			EventID:    memberEvent.EventID,
		}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("create membership: %w", err)
		}

		// 9. Room alias (if requested)
		if params.RoomAlias != "" {
			alias := fmt.Sprintf("#%s:%s", strings.TrimPrefix(params.RoomAlias, "#"), s.Domain)
			roomAlias := storage.RoomAlias{
				Alias:   alias,
				RoomID:  roomID,
				Creator: params.Creator,
			}
			if err := tx.Create(&roomAlias).Error; err != nil {
				return fmt.Errorf("create room alias: %w", err)
			}
		}

		// 10. Current room state for initial events
		stateInserts := []storage.CurrentRoomState{
			{RoomID: roomID, EventType: "m.room.create", StateKey: "", EventID: createEvent.EventID, Content: createEvent.Content.Raw},
			{RoomID: roomID, EventType: "m.room.member", StateKey: params.Creator, EventID: memberEvent.EventID, Content: memberEvent.Content.Raw},
			{RoomID: roomID, EventType: "m.room.power_levels", StateKey: "", EventID: plEvent.EventID, Content: plEvent.Content.Raw},
			{RoomID: roomID, EventType: "m.room.join_rules", StateKey: "", EventID: jrEvent.EventID, Content: jrEvent.Content.Raw},
		}
		if params.Name != "" {
			stateInserts = append(stateInserts, storage.CurrentRoomState{RoomID: roomID, EventType: "m.room.name", StateKey: "", Content: storage.JSONMap{"name": params.Name}})
		}
		if params.Topic != "" {
			stateInserts = append(stateInserts, storage.CurrentRoomState{RoomID: roomID, EventType: "m.room.topic", StateKey: "", Content: storage.JSONMap{"topic": params.Topic}})
		}
		for _, si := range stateInserts {
			if err := tx.Where("room_id = ? AND event_type = ? AND state_key = ?", si.RoomID, si.EventType, si.StateKey).
				Assign(si).FirstOrCreate(&si).Error; err != nil {
				return fmt.Errorf("upsert current_state: %w", err)
			}
		}

		// 11. Invites
		for _, invitee := range params.Invite {
			inviteContent := storage.EventContent{
				Membership: "invite",
				Raw:        storage.JSONMap{"membership": "invite"},
			}
			inviteEvent := s.newEvent(roomID, params.Creator, "m.room.member", invitee, inviteContent, originTS, depth, frontier, nil)
			if err := tx.Create(&inviteEvent).Error; err != nil {
				return fmt.Errorf("create invite for %s: %w", invitee, err)
			}
			inviteMembership := storage.RoomMembership{
				RoomID:     roomID,
				UserID:     invitee,
				Sender:     params.Creator,
				Membership: "invite",
				EventID:    inviteEvent.EventID,
			}
			if err := tx.Where("room_id = ? AND user_id = ?", roomID, invitee).
				Assign(inviteMembership).FirstOrCreate(&inviteMembership).Error; err != nil {
				return fmt.Errorf("create invite membership: %w", err)
			}
			frontier = []string{inviteEvent.EventID}
			depth++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &CreateRoomResult{RoomID: roomID}, nil
}

// ---------------------------------------------------------------------------
// Membership operations
// ---------------------------------------------------------------------------

// JoinRoom joins a user to a room (or accepts an invite).
func (s *Service) JoinRoom(userID, roomID string) (string, error) {
	// Verify room exists
	var room storage.Room
	if err := s.DB.Where("room_id = ?", roomID).First(&room).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("room %s not found", roomID)
		}
		return "", err
	}

	// Check currently joined
	var existing storage.RoomMembership
	err := s.DB.Where("room_id = ? AND user_id = ?", roomID, userID).First(&existing).Error
	if err == nil && existing.Membership == "join" {
		// Already joined — return room_id (idempotent)
		return roomID, nil
	}

	eventID := generateEventID(s.Domain)
	originTS := time.Now().UnixMilli()

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		// Determine depth
		var maxDepth int64
		tx.Model(&storage.Event{}).Where("room_id = ?", roomID).
			Select("COALESCE(MAX(depth), 0)").Scan(&maxDepth)
		depth := maxDepth + 1

		content := storage.EventContent{
			Membership: "join",
			Raw:        storage.JSONMap{"membership": "join"},
		}

		ev := s.newEvent(roomID, userID, "m.room.member", userID, content, originTS, depth, nil, nil)
		ev.EventID = eventID
		if err := tx.Create(&ev).Error; err != nil {
			return fmt.Errorf("create join event: %w", err)
		}

		// Upsert membership
		m := storage.RoomMembership{
			RoomID:     roomID,
			UserID:     userID,
			Sender:     userID,
			Membership: "join",
			EventID:    eventID,
		}
		if err := tx.Where("room_id = ? AND user_id = ?", roomID, userID).
			Assign(m).FirstOrCreate(&m).Error; err != nil {
			return fmt.Errorf("upsert membership: %w", err)
		}

		// Update current room state
		cs := storage.CurrentRoomState{
			RoomID:    roomID,
			EventType: "m.room.member",
			StateKey:  userID,
			EventID:   eventID,
			Content:   content.Raw,
		}
		return tx.Where("room_id = ? AND event_type = ? AND state_key = ?", roomID, "m.room.member", userID).
			Assign(cs).FirstOrCreate(&cs).Error
	})
	if err != nil {
		return "", err
	}

	return roomID, nil
}

// InviteUser creates an invite for a target user.
func (s *Service) InviteUser(roomID, senderID, targetID, reason string) (*InviteResult, error) {
	// Verify sender is joined
	var senderMembership storage.RoomMembership
	if err := s.DB.Where("room_id = ? AND user_id = ? AND membership = ?", roomID, senderID, "join").First(&senderMembership).Error; err != nil {
		return nil, fmt.Errorf("sender %s is not a member of room %s", senderID, roomID)
	}

	eventID := generateEventID(s.Domain)
	originTS := time.Now().UnixMilli()

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var maxDepth int64
		tx.Model(&storage.Event{}).Where("room_id = ?", roomID).
			Select("COALESCE(MAX(depth), 0)").Scan(&maxDepth)
		depth := maxDepth + 1

		content := storage.EventContent{
			Membership: "invite",
			Reason:     reason,
			Raw:        storage.JSONMap{"membership": "invite"},
		}

		ev := s.newEvent(roomID, senderID, "m.room.member", targetID, content, originTS, depth, nil, nil)
		ev.EventID = eventID
		if err := tx.Create(&ev).Error; err != nil {
			return fmt.Errorf("create invite event: %w", err)
		}

		m := storage.RoomMembership{
			RoomID:     roomID,
			UserID:     targetID,
			Sender:     senderID,
			Membership: "invite",
			Reason:     reason,
			EventID:    eventID,
		}
		return tx.Where("room_id = ? AND user_id = ?", roomID, targetID).
			Assign(m).FirstOrCreate(&m).Error
	})
	if err != nil {
		return nil, err
	}

	return &InviteResult{RoomID: roomID, InvitedUserID: targetID}, nil
}

// LeaveRoom removes the user from the room.
func (s *Service) LeaveRoom(userID, roomID string) error {
	return s.setMembership(userID, roomID, "leave", "")
}

// KickUser removes a user from a room (must be called by a moderator).
func (s *Service) KickUser(senderID, roomID, targetID, reason string) error {
	// Verify sender is still joined
	var senderMembership storage.RoomMembership
	if err := s.DB.Where("room_id = ? AND user_id = ? AND membership = ?", roomID, senderID, "join").First(&senderMembership).Error; err != nil {
		return fmt.Errorf("sender is not a member of the room")
	}
	return s.setMembership(targetID, roomID, "leave", reason)
}

// BanUser bans a target user.
func (s *Service) BanUser(senderID, roomID, targetID, reason string) error {
	return s.setMembership(targetID, roomID, "ban", reason)
}

// UnbanUser lifts a ban.
func (s *Service) UnbanUser(senderID, roomID, targetID string) error {
	return s.setMembership(targetID, roomID, "leave", "")
}

// setMembership is the internal helper for changing membership state.
func (s *Service) setMembership(userID, roomID, membership, reason string) error {
	eventID := generateEventID(s.Domain)
	originTS := time.Now().UnixMilli()

	return s.DB.Transaction(func(tx *gorm.DB) error {
		var maxDepth int64
		tx.Model(&storage.Event{}).Where("room_id = ?", roomID).
			Select("COALESCE(MAX(depth), 0)").Scan(&maxDepth)
		depth := maxDepth + 1

		content := storage.EventContent{
			Membership: membership,
			Reason:     reason,
			Raw:        storage.JSONMap{"membership": membership},
		}

		ev := s.newEvent(roomID, userID, "m.room.member", userID, content, originTS, depth, nil, nil)
		ev.EventID = eventID
		if err := tx.Create(&ev).Error; err != nil {
			return fmt.Errorf("create membership event: %w", err)
		}

		if err := tx.Where("room_id = ? AND user_id = ?", roomID, userID).
			Assign(storage.RoomMembership{Membership: membership, Sender: userID, EventID: eventID, Reason: reason}).
			FirstOrCreate(&storage.RoomMembership{}).Error; err != nil {
			return fmt.Errorf("upsert membership: %w", err)
		}

		// Update current room state
		cs := storage.CurrentRoomState{
			RoomID:    roomID,
			EventType: "m.room.member",
			StateKey:  userID,
			EventID:   eventID,
			Content:   content.Raw,
		}
		return tx.Where("room_id = ? AND event_type = ? AND state_key = ?", roomID, "m.room.member", userID).
			Assign(cs).FirstOrCreate(&cs).Error
	})
}

// ---------------------------------------------------------------------------
// Messages & events
// ---------------------------------------------------------------------------

// SendMessage persists a room message event (m.room.message).
func (s *Service) SendMessage(roomID, senderID, msgType, body, formattedBody, format string, txnID string) (string, error) {
	// Verify membership
	var m storage.RoomMembership
	if err := s.DB.Where("room_id = ? AND user_id = ? AND membership = ?", roomID, senderID, "join").First(&m).Error; err != nil {
		return "", fmt.Errorf("user %s is not joined to room %s", senderID, roomID)
	}

	eventID := generateEventID(s.Domain)
	originTS := time.Now().UnixMilli()

	content := storage.EventContent{
		Body:          body,
		MsgType:       msgType,
		Format:        format,
		FormattedBody: formattedBody,
	}

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var maxDepth int64
		tx.Model(&storage.Event{}).Where("room_id = ?", roomID).
			Select("COALESCE(MAX(depth), 0)").Scan(&maxDepth)
		depth := maxDepth + 1

		ev := s.newEvent(roomID, senderID, "m.room.message", "", content, originTS, depth, nil, nil)
		ev.EventID = eventID
		return tx.Create(&ev).Error
	})
	if err != nil {
		return "", fmt.Errorf("failed to persist message: %w", err)
	}

	return eventID, nil
}

// GetMessages retrieves paginated messages from a room.
func (s *Service) GetMessages(roomID, from, dir string, limit int) ([]storage.Event, string, string, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}

	var events []storage.Event
	tx := s.DB.Where("room_id = ? AND event_type = ?", roomID, "m.room.message")

	if dir == "b" || dir == "" {
		if from != "" {
			tx = tx.Where("origin_server_ts < (SELECT origin_server_ts FROM events WHERE event_id = ?)", from)
		}
		tx = tx.Order("origin_server_ts DESC, event_id DESC").Limit(limit)
	} else {
		if from != "" {
			tx = tx.Where("origin_server_ts > (SELECT origin_server_ts FROM events WHERE event_id = ?)", from)
		}
		tx = tx.Order("origin_server_ts ASC, event_id ASC").Limit(limit)
	}

	if err := tx.Find(&events).Error; err != nil {
		return nil, "", "", fmt.Errorf("query messages: %w", err)
	}

	var start, end string
	if len(events) > 0 {
		start = events[0].EventID
		end = events[len(events)-1].EventID
	}

	return events, start, end, nil
}

// GetState returns the current state for a room.
func (s *Service) GetState(roomID string) ([]storage.CurrentRoomState, error) {
	var state []storage.CurrentRoomState
	err := s.DB.Where("room_id = ?", roomID).Find(&state).Error
	return state, err
}

// IsMember checks if a user is joined to a room.
func (s *Service) IsMember(roomID, userID string) bool {
	var count int64
	s.DB.Model(&storage.RoomMembership{}).
		Where("room_id = ? AND user_id = ? AND membership = ?", roomID, userID, "join").
		Count(&count)
	return count > 0
}

// GetInvitedRooms returns rooms where the user has a pending invite.
func (s *Service) GetInvitedRooms(userID string) ([]storage.RoomMembership, error) {
	var memberships []storage.RoomMembership
	err := s.DB.Where("user_id = ? AND membership = ?", userID, "invite").Find(&memberships).Error
	return memberships, err
}

// GetJoinedRooms returns the list of room IDs the user is currently joined to.
func (s *Service) GetJoinedRooms(userID string) ([]string, error) {
	var memberships []storage.RoomMembership
	if err := s.DB.Where("user_id = ? AND membership = ?", userID, "join").Find(&memberships).Error; err != nil {
		return nil, err
	}
	roomIDs := make([]string, len(memberships))
	for i, m := range memberships {
		roomIDs[i] = m.RoomID
	}
	return roomIDs, nil
}

// GetRoomMembers returns all members of a room and their membership states.
func (s *Service) GetRoomMembers(roomID string) ([]storage.RoomMembership, error) {
	var memberships []storage.RoomMembership
	err := s.DB.Where("room_id = ?", roomID).Find(&memberships).Error
	return memberships, err
}

// ---------------------------------------------------------------------------
// Sync helpers
// ---------------------------------------------------------------------------

// GetEventsSince returns events in a room after a given stream position.
func (s *Service) GetEventsSince(roomID string, since int64, limit int) ([]storage.Event, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var events []storage.Event
	if err := s.DB.Where("room_id = ? AND id > ?", roomID, since).
		Order("id ASC").Limit(limit).Find(&events).Error; err != nil {
		return nil, 0, err
	}

	var nextBatch int64
	if len(events) > 0 {
		nextBatch = int64(events[len(events)-1].ID)
	} else {
		nextBatch = since
	}

	return events, nextBatch, nil
}

// MaxStreamPosition returns the highest event ID (used as sync token).
func (s *Service) MaxStreamPosition() int64 {
	var maxID int64
	s.DB.Model(&storage.Event{}).Select("COALESCE(MAX(id), 0)").Scan(&maxID)
	return maxID
}

// ---------------------------------------------------------------------------
// Redact, state, forget
// ---------------------------------------------------------------------------

// RedactEvent creates a redaction event (m.room.redaction) for the target event.
func (s *Service) RedactEvent(senderID, roomID, eventID, reason string) (string, error) {
	var target storage.Event
	if err := s.DB.Where("event_id = ? AND room_id = ?", eventID, roomID).First(&target).Error; err != nil {
		return "", fmt.Errorf("target event not found: %w", err)
	}

	newEventID := generateEventID(s.Domain)
	originTS := time.Now().UnixMilli()

	content := storage.EventContent{
		Reason: reason,
		Raw:    storage.JSONMap{"reason": reason},
	}

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var maxDepth int64
		tx.Model(&storage.Event{}).Where("room_id = ?", roomID).
			Select("COALESCE(MAX(depth), 0)").Scan(&maxDepth)
		depth := maxDepth + 1

		ev := s.newEvent(roomID, senderID, "m.room.redaction", "", content, originTS, depth, nil, nil)
		ev.EventID = newEventID
		ev.Redacts = eventID
		return tx.Create(&ev).Error
	})
	if err != nil {
		return "", fmt.Errorf("failed to create redaction: %w", err)
	}

	// Mark original event as redacted
	s.DB.Model(&storage.Event{}).Where("event_id = ?", eventID).
		Update("unsigned", storage.JSONMap{"redacted_because": map[string]interface{}{
			"event_id": newEventID,
			"reason":   reason,
			"sender":   senderID,
		}})

	return newEventID, nil
}

// SetStateEvent directly sets a state event in a room.
func (s *Service) SetStateEvent(roomID, senderID, eventType, stateKey string, contentJSON []byte) (string, error) {
	eventID := generateEventID(s.Domain)
	originTS := time.Now().UnixMilli()

	content := storage.EventContent{
		Raw: storage.JSONMap{"content": string(contentJSON)},
	}

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var maxDepth int64
		tx.Model(&storage.Event{}).Where("room_id = ?", roomID).
			Select("COALESCE(MAX(depth), 0)").Scan(&maxDepth)
		depth := maxDepth + 1

		ev := s.newEvent(roomID, senderID, eventType, stateKey, content, originTS, depth, nil, nil)
		ev.EventID = eventID
		if err := tx.Create(&ev).Error; err != nil {
			return err
		}

		// Update current room state
		cs := storage.CurrentRoomState{
			RoomID:    roomID,
			EventType: eventType,
			StateKey:  stateKey,
			EventID:   eventID,
			Content:   storage.JSONMap{"content": string(contentJSON)},
		}
		return tx.Where("room_id = ? AND event_type = ? AND state_key = ?", roomID, eventType, stateKey).
			Assign(cs).FirstOrCreate(&cs).Error
	})
	if err != nil {
		return "", err
	}

	return eventID, nil
}

// ForgetRoom removes a user's membership entirely (after leaving).
func (s *Service) ForgetRoom(userID, roomID string) error {
	return s.DB.Where("room_id = ? AND user_id = ?", roomID, userID).
		Delete(&storage.RoomMembership{}).Error
}

// ---------------------------------------------------------------------------
// Event construction helpers
// ---------------------------------------------------------------------------

// newEvent builds a storage.Event with the given parameters.
func (s *Service) newEvent(roomID, sender, eventType, stateKey string, content storage.EventContent, originTS, depth int64, prevEventIDs, authEventIDs []string) storage.Event {
	var stateKeyPtr *string
	if stateKey != "" {
		stateKeyPtr = &stateKey
	}

	eventID := generateEventID(s.Domain)

	return storage.Event{
		EventID:        eventID,
		RoomID:         roomID,
		Sender:         sender,
		EventType:      eventType,
		StateKey:       stateKeyPtr,
		Content:        content,
		OriginServerTS: originTS,
		Depth:          depth,
	}
}

// ---------------------------------------------------------------------------
// ID generation
// ---------------------------------------------------------------------------

func generateRoomID(serverName string) string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return fmt.Sprintf("!%s:%s", hex.EncodeToString(b), serverName)
}

func generateEventID(serverName string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return fmt.Sprintf("$%s:%s", hex.EncodeToString(b), serverName)
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

func defaultPowerLevels(creator string) storage.JSONMap {
	return storage.JSONMap{
		"users": storage.JSONMap{
			creator: float64(100),
		},
		"users_default":    float64(0),
		"events":           storage.JSONMap{"m.room.name": float64(50), "m.room.power_levels": float64(100), "m.room.history_visibility": float64(100), "m.room.canonical_alias": float64(50), "m.room.avatar": float64(50), "m.room.tombstone": float64(100), "m.room.server_acl": float64(100), "m.room.encryption": float64(100)},
		"events_default":   float64(0),
		"state_default":    float64(50),
		"ban":              float64(50),
		"kick":             float64(50),
		"redact":           float64(50),
		"invite":           float64(0),
		"historical":       float64(100),
	}
}
