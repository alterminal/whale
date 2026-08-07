package client

import "encoding/json"

// This file contains only pure data types (DTOs) for Matrix C-S API
// request/response payloads. Handler method logic lives in router.go
// and the handler_*.go files.

// ---------------------------------------------------------------------------
// Versions / Capabilities
// ---------------------------------------------------------------------------

type VersionsResponse struct {
	Versions         []string        `json:"versions"`
	UnstableFeatures map[string]bool `json:"unstable_features,omitempty"`
}

type CapabilitiesResponse struct {
	Capabilities Capabilities `json:"capabilities"`
}

type Capabilities struct {
	ChangePassword *CapBool         `json:"m.change_password,omitempty"`
	RoomVersions   *RoomVersionsCap `json:"m.room_versions,omitempty"`
}

type CapBool struct {
	Enabled bool `json:"enabled"`
}

type RoomVersionsCap struct {
	Default   string   `json:"default"`
	Available []string `json:"available"`
}

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

type LoginRequest struct {
	Type                     string `json:"type"`
	Identifier               *LoginIdentifier `json:"identifier,omitempty"`
	User                     string `json:"user,omitempty"`
	Password                 string `json:"password,omitempty"`
	Token                    string `json:"token,omitempty"`
	DeviceID                 string `json:"device_id,omitempty"`
	InitialDeviceDisplayName string `json:"initial_device_display_name,omitempty"`
}

type LoginIdentifier struct {
	Type string `json:"type"`
	User string `json:"user,omitempty"`
}

type LoginResponse struct {
	UserID      string            `json:"user_id"`
	AccessToken string            `json:"access_token"`
	DeviceID    string            `json:"device_id"`
	HomeServer  string            `json:"home_server,omitempty"`
	WellKnown   *WellKnownResponse `json:"well_known,omitempty"`
}

type WellKnownResponse struct {
	HomeServer WellKnownBaseURL `json:"m.homeserver"`
}

type WellKnownBaseURL struct {
	BaseURL string `json:"base_url"`
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

type RegisterRequest struct {
	Username                 string    `json:"username"`
	Password                 string    `json:"password"`
	DeviceID                 string    `json:"device_id,omitempty"`
	InitialDeviceDisplayName string    `json:"initial_device_display_name,omitempty"`
	InhibitLogin             bool      `json:"inhibit_login,omitempty"`
	Auth                     *AuthDict `json:"auth,omitempty"`
}

type RegisterResponse struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token,omitempty"`
	DeviceID    string `json:"device_id,omitempty"`
	HomeServer  string `json:"home_server"`
}

type AuthDict struct {
	Type         string         `json:"type"`
	Session      string         `json:"session,omitempty"`
	ThreepidCreds *ThreePIDCreds `json:"threepid_creds,omitempty"`
}

type ThreePIDCreds struct {
	ClientSecret string `json:"client_secret"`
	Sid          string `json:"sid"`
}

// ---------------------------------------------------------------------------
// WhoAmI
// ---------------------------------------------------------------------------

type WhoAmIResponse struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id,omitempty"`
	IsGuest  bool   `json:"is_guest"`
}

// ---------------------------------------------------------------------------
// CreateRoom
// ---------------------------------------------------------------------------

type CreateRoomRequest struct {
	Visibility  string   `json:"visibility,omitempty"`
	RoomAlias   string   `json:"room_alias_name,omitempty"`
	Name        string   `json:"name,omitempty"`
	Topic       string   `json:"topic,omitempty"`
	Invite      []string `json:"invite,omitempty"`
	Preset      string   `json:"preset,omitempty"`
	IsDirect    bool     `json:"is_direct,omitempty"`
	RoomVersion string   `json:"room_version,omitempty"`
}

type CreateRoomResponse struct {
	RoomID string `json:"room_id"`
}

// ---------------------------------------------------------------------------
// Room membership
// ---------------------------------------------------------------------------

type JoinRequest struct {
	Reason string `json:"reason,omitempty"`
}

type InviteRequest struct {
	UserID string `json:"user_id"`
	Reason string `json:"reason,omitempty"`
}

type KickRequest struct {
	UserID string `json:"user_id"`
	Reason string `json:"reason,omitempty"`
}

type BanRequest struct {
	UserID string `json:"user_id"`
	Reason string `json:"reason,omitempty"`
}

type UnbanRequest struct {
	UserID string `json:"user_id"`
}

type LeaveRequest struct {
	Reason string `json:"reason,omitempty"`
}

type JoinResponse struct {
	RoomID string `json:"room_id"`
}

type InviteResponse struct {
	RoomID string `json:"room_id"`
}

type EmptyResponse struct{}

// ---------------------------------------------------------------------------
// Joined Rooms
// ---------------------------------------------------------------------------

type JoinedRoomsResponse struct {
	JoinedRooms []string `json:"joined_rooms"`
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

type StateEventResponse struct {
	EventType string          `json:"type"`
	StateKey  string          `json:"state_key"`
	Content   json.RawMessage `json:"content"`
	EventID   string          `json:"event_id"`
}

// ---------------------------------------------------------------------------
// Send Event
// ---------------------------------------------------------------------------

type SendEventResponse struct {
	EventID string `json:"event_id"`
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type MessagesResponse struct {
	Start string     `json:"start"`
	End   string     `json:"end"`
	Chunk []EventDTO `json:"chunk"`
}

// EventDTO is the JSON representation of a Matrix event in API responses.
type EventDTO struct {
	EventID        string          `json:"event_id"`
	RoomID         string          `json:"room_id"`
	Sender         string          `json:"sender"`
	EventType      string          `json:"type"`
	StateKey       *string         `json:"state_key,omitempty"`
	Content        json.RawMessage `json:"content"`
	OriginServerTS int64           `json:"origin_server_ts"`
	Unsigned       json.RawMessage `json:"unsigned,omitempty"`
	Redacts        string          `json:"redacts,omitempty"`
}

// ---------------------------------------------------------------------------
// Sync
// ---------------------------------------------------------------------------

type SyncResponse struct {
	NextBatch   string              `json:"next_batch"`
	Rooms       *SyncRooms          `json:"rooms,omitempty"`
	Presence    *SyncPresence       `json:"presence,omitempty"`
	AccountData *SyncAccountData    `json:"account_data,omitempty"`
	DeviceLists *DeviceLists        `json:"device_lists,omitempty"`
	DeviceOneTimeKeysCount map[string]int `json:"device_one_time_keys_count,omitempty"`
}

type SyncRooms struct {
	Join   map[string]JoinedRoom  `json:"join,omitempty"`
	Invite map[string]InvitedRoom `json:"invite,omitempty"`
	Leave  map[string]LeftRoom    `json:"leave,omitempty"`
}

type JoinedRoom struct {
	State                *StateSection      `json:"state,omitempty"`
	Timeline             *TimelineSection   `json:"timeline"`
	Ephemeral            *EphemeralSection  `json:"ephemeral,omitempty"`
	AccountData          *RoomAccountSection `json:"account_data,omitempty"`
	UnreadNotifications  *UnreadCount       `json:"unread_notifications,omitempty"`
}

type InvitedRoom struct {
	InviteState *InviteStateSection `json:"invite_state"`
}

type LeftRoom struct {
	State    *StateSection    `json:"state,omitempty"`
	Timeline *TimelineSection `json:"timeline"`
}

type StateSection struct {
	Events []EventDTO `json:"events"`
}

type TimelineSection struct {
	Events    []EventDTO `json:"events"`
	Limited   bool       `json:"limited"`
	PrevBatch string     `json:"prev_batch,omitempty"`
}

type EphemeralSection struct {
	Events []json.RawMessage `json:"events"`
}

type RoomAccountSection struct {
	Events []json.RawMessage `json:"events"`
}

type InviteStateSection struct {
	Events []EventDTO `json:"events"`
}

type UnreadCount struct {
	HighlightCount    int `json:"highlight_count"`
	NotificationCount int `json:"notification_count"`
}

type SyncPresence struct {
	Events []PresenceEvent `json:"events"`
}

type PresenceEvent struct {
	Sender          string `json:"sender"`
	Presence        string `json:"presence"`
	StatusMessage   string `json:"status_msg,omitempty"`
	CurrentlyActive bool   `json:"currently_active,omitempty"`
	LastActiveAgo   int64  `json:"last_active_ago,omitempty"`
}

type SyncAccountData struct {
	Events []json.RawMessage `json:"events"`
}

type DeviceLists struct {
	Changed []string `json:"changed,omitempty"`
	Left    []string `json:"left,omitempty"`
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

type ProfileResponse struct {
	DisplayName string `json:"displayname,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

type DisplayNameRequest struct {
	DisplayName string `json:"displayname"`
}

type AvatarURLRequest struct {
	AvatarURL string `json:"avatar_url"`
}

// ---------------------------------------------------------------------------
// Filter
// ---------------------------------------------------------------------------

type FilterRequest struct {
	AccountData *FilterSection `json:"account_data,omitempty"`
	Presence    *FilterSection `json:"presence,omitempty"`
	Room        *RoomFilter    `json:"room,omitempty"`
}

type FilterSection struct {
	Limit      int      `json:"limit"`
	NotSenders []string `json:"not_senders,omitempty"`
	NotTypes   []string `json:"not_types,omitempty"`
	Senders    []string `json:"senders,omitempty"`
	Types      []string `json:"types,omitempty"`
}

type RoomFilter struct {
	NotRooms     []string       `json:"not_rooms,omitempty"`
	Rooms        []string       `json:"rooms,omitempty"`
	Ephemeral    *FilterSection `json:"ephemeral,omitempty"`
	IncludeLeave bool           `json:"include_leave,omitempty"`
	State        *StateFilter   `json:"state,omitempty"`
	Timeline     *StateFilter   `json:"timeline,omitempty"`
	AccountData  *FilterSection `json:"account_data,omitempty"`
}

type StateFilter struct {
	Limit                     int      `json:"limit"`
	NotSenders                []string `json:"not_senders,omitempty"`
	NotTypes                  []string `json:"not_types,omitempty"`
	Senders                   []string `json:"senders,omitempty"`
	Types                     []string `json:"types,omitempty"`
	NotRooms                  []string `json:"not_rooms,omitempty"`
	Rooms                     []string `json:"rooms,omitempty"`
	ContainsURL               *bool    `json:"contains_url,omitempty"`
	IncludeRedundantMembers   bool     `json:"include_redundant_members,omitempty"`
	LazyLoadMembers           bool     `json:"lazy_load_members,omitempty"`
	UnreadThreadNotifications bool     `json:"unread_thread_notifications,omitempty"`
}

type FilterResponse struct {
	FilterID string `json:"filter_id"`
}

// ---------------------------------------------------------------------------
// E2EE Keys
// ---------------------------------------------------------------------------

type KeyUploadRequest struct {
	DeviceKeys   *DeviceKeysUpload           `json:"device_keys,omitempty"`
	OneTimeKeys  map[string]interface{}      `json:"one_time_keys,omitempty"`
	FallbackKeys map[string]interface{}      `json:"fallback_keys,omitempty"`
}

type DeviceKeysUpload struct {
	UserID     string                       `json:"user_id"`
	DeviceID   string                       `json:"device_id"`
	Algorithms []string                     `json:"algorithms"`
	Keys       map[string]string            `json:"keys"`
	Signatures map[string]map[string]string `json:"signatures"`
}

type KeyUploadResponse struct {
	OneTimeKeyCounts map[string]int `json:"one_time_key_counts"`
}

type KeyQueryRequest struct {
	DeviceKeys map[string][]string `json:"device_keys"`
	Timeout    int                 `json:"timeout,omitempty"`
}

type KeyQueryResponse struct {
	DeviceKeys map[string]map[string]DeviceKeyDTO `json:"device_keys"`
	Failures   map[string]interface{}             `json:"failures,omitempty"`
}

type DeviceKeyDTO struct {
	UserID     string                       `json:"user_id"`
	DeviceID   string                       `json:"device_id"`
	Algorithms []string                     `json:"algorithms"`
	Keys       map[string]string            `json:"keys"`
	Signatures map[string]map[string]string `json:"signatures"`
	Unsigned   map[string]interface{}       `json:"unsigned,omitempty"`
}

type KeyClaimRequest struct {
	OneTimeKeys map[string]map[string]string `json:"one_time_keys"`
	Timeout     int                          `json:"timeout,omitempty"`
}

type KeyClaimResponse struct {
	OneTimeKeys map[string]map[string]interface{} `json:"one_time_keys"`
	Failures    map[string]interface{}            `json:"failures,omitempty"`
}

// ---------------------------------------------------------------------------
// Presence
// ---------------------------------------------------------------------------

type PresenceRequest struct {
	Presence      string `json:"presence"`
	StatusMessage string `json:"status_msg,omitempty"`
}

type PresenceStatusResponse struct {
	Presence        string `json:"presence"`
	StatusMessage   string `json:"status_msg,omitempty"`
	CurrentlyActive bool   `json:"currently_active,omitempty"`
	LastActiveAgo   int64  `json:"last_active_ago,omitempty"`
}

// ---------------------------------------------------------------------------
// Members
// ---------------------------------------------------------------------------

type MembersResponse struct {
	Chunk []MemberEventDTO `json:"chunk"`
}

type MemberEventDTO struct {
	EventID        string        `json:"event_id"`
	Sender         string        `json:"sender"`
	EventType      string        `json:"type"`
	StateKey       string        `json:"state_key"`
	Content        MemberContent `json:"content"`
	OriginServerTS int64         `json:"origin_server_ts"`
}

type MemberContent struct {
	Membership  string `json:"membership"`
	DisplayName string `json:"displayname,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// ---------------------------------------------------------------------------
// Room Alias
// ---------------------------------------------------------------------------

type RoomAliasResponse struct {
	RoomID  string   `json:"room_id"`
	Servers []string `json:"servers"`
}

type RoomAliasRequest struct {
	RoomID string `json:"room_id"`
}

// ---------------------------------------------------------------------------
// Media
// ---------------------------------------------------------------------------

type MediaUploadResponse struct {
	ContentURI string `json:"content_uri"`
}

// ---------------------------------------------------------------------------
// .well-known
// ---------------------------------------------------------------------------

// WellKnownClientResponse is the JSON returned by /.well-known/matrix/client.
// https://spec.matrix.org/latest/client-server-api/#getwell-knownmatrixclient
type WellKnownClientResponse struct {
	HomeServer     WellKnownBaseURL             `json:"m.homeserver"`
	IdentityServer *WellKnownBaseURL            `json:"m.identity_server,omitempty"`
	Extra          map[string]json.RawMessage   `json:"-"`
}

// WellKnownServerResponse is the JSON returned by /.well-known/matrix/server.
// https://spec.matrix.org/latest/server-server-api/#getwell-knownmatrixserver
type WellKnownServerResponse struct {
	Server string `json:"m.server"`
}
