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

// LoginFlowsResponse is returned by GET /login to advertise supported auth flows.
type LoginFlowsResponse struct {
	Flows []LoginFlow `json:"flows"`
}

type LoginFlow struct {
	Type string `json:"type"`
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
// Account Data
// ---------------------------------------------------------------------------

type AccountDataResponse struct {
	// Content is raw JSON depending on the data type.
	Content json.RawMessage `json:"-"`
}

// ---------------------------------------------------------------------------
// Password change
// ---------------------------------------------------------------------------

type PasswordRequest struct {
	NewPassword string `json:"new_password"`
	LogoutDevices bool `json:"logout_devices,omitempty"`
	Auth        *AuthDict `json:"auth,omitempty"`
}

// ---------------------------------------------------------------------------
// Deactivate
// ---------------------------------------------------------------------------

type DeactivateRequest struct {
	Auth *AuthDict `json:"auth,omitempty"`
	Erase bool     `json:"erase,omitempty"`
}

// ---------------------------------------------------------------------------
// Refresh token
// ---------------------------------------------------------------------------

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresInMs  int64  `json:"expires_in_ms,omitempty"`
}

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

type DeviceDTO struct {
	DeviceID    string `json:"device_id"`
	DisplayName string `json:"display_name,omitempty"`
	LastSeenIP  string `json:"last_seen_ip,omitempty"`
	LastSeenTS  int64  `json:"last_seen_ts,omitempty"`
}

type DevicesResponse struct {
	Devices []DeviceDTO `json:"devices"`
}

type DeviceUpdateRequest struct {
	DisplayName string `json:"display_name"`
}

type DeleteDevicesRequest struct {
	Devices []string `json:"devices"`
}

// ---------------------------------------------------------------------------
// Redact
// ---------------------------------------------------------------------------

type RedactRequest struct {
	Reason string `json:"reason,omitempty"`
}

// ---------------------------------------------------------------------------
// Typing
// ---------------------------------------------------------------------------

type TypingRequest struct {
	Typing  bool   `json:"typing"`
	Timeout int    `json:"timeout,omitempty"`
}

// ---------------------------------------------------------------------------
// Receipt
// ---------------------------------------------------------------------------

type ReceiptRequest struct {
	// Usually an empty JSON body; receiptType and eventId are in the path.
}

// ---------------------------------------------------------------------------
// Event context
// ---------------------------------------------------------------------------

type EventContextResponse struct {
	Start        string     `json:"start"`
	End          string     `json:"end"`
	EventsBefore []EventDTO `json:"events_before"`
	Event        *EventDTO  `json:"event"`
	EventsAfter  []EventDTO `json:"events_after"`
	State        []EventDTO `json:"state"`
}

// ---------------------------------------------------------------------------
// Joined members (lightweight)
// ---------------------------------------------------------------------------

type JoinedMembersResponse struct {
	Joined map[string]JoinedMemberInfo `json:"joined"`
}

type JoinedMemberInfo struct {
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// ---------------------------------------------------------------------------
// Public Rooms
// ---------------------------------------------------------------------------

type PublicRoomsRequest struct {
	Limit                int    `json:"limit,omitempty"`
	Since                string `json:"since,omitempty"`
	Filter               *PublicRoomsFilter `json:"filter,omitempty"`
	IncludeAllNetworks   bool   `json:"include_all_networks,omitempty"`
	ThirdPartyInstanceID string `json:"third_party_instance_id,omitempty"`
}

type PublicRoomsFilter struct {
	GenericSearchTerm string `json:"generic_search_term,omitempty"`
	RoomTypes         []string `json:"room_types,omitempty"`
}

type PublicRoomsResponse struct {
	Chunk           []PublicRoomDTO `json:"chunk"`
	NextBatch       string          `json:"next_batch,omitempty"`
	PrevBatch       string          `json:"prev_batch,omitempty"`
	TotalRoomCountEstimate int      `json:"total_room_count_estimate,omitempty"`
}

type PublicRoomDTO struct {
	RoomID           string   `json:"room_id"`
	Name             string   `json:"name,omitempty"`
	Topic            string   `json:"topic,omitempty"`
	CanonicalAlias   string   `json:"canonical_alias,omitempty"`
	Aliases          []string `json:"aliases,omitempty"`
	NumJoinedMembers int      `json:"num_joined_members"`
	WorldReadable    bool     `json:"world_readable"`
	GuestCanJoin     bool     `json:"guest_can_join"`
	AvatarURL        string   `json:"avatar_url,omitempty"`
	JoinRule         string   `json:"join_rule,omitempty"`
	RoomType         string   `json:"room_type,omitempty"`
}

// ---------------------------------------------------------------------------
// Directory visibility
// ---------------------------------------------------------------------------

type RoomVisibilityRequest struct {
	Visibility string `json:"visibility"` // "public" or "private"
}

type RoomVisibilityResponse struct {
	Visibility string `json:"visibility"`
}

// ---------------------------------------------------------------------------
// Room Tags
// ---------------------------------------------------------------------------

type TagRequest struct {
	Order    json.RawMessage `json:"order,omitempty"`   // float between 0 and 1
	AdditionalProperties map[string]interface{} `json:"-"` // custom per-tag data
}

type TagsResponse struct {
	Tags map[string]TagContent `json:"tags"`
}

type TagContent struct {
	Order json.RawMessage `json:"order,omitempty"`
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

type SearchRequest struct {
	SearchCategories SearchCategories `json:"search_categories"`
}

type SearchCategories struct {
	RoomEvents *RoomEventsSearch `json:"room_events,omitempty"`
}

type RoomEventsSearch struct {
	SearchTerm string       `json:"search_term"`
	Keys       []string     `json:"keys,omitempty"`
	Filter     *RoomFilter  `json:"filter,omitempty"`
	OrderBy    string       `json:"order_by,omitempty"`
	EventContext *EventContextFilter `json:"event_context,omitempty"`
	IncludeState    bool     `json:"include_state,omitempty"`
	Groupings    *Groupings  `json:"groupings,omitempty"`
}

type EventContextFilter struct {
	BeforeLimit int  `json:"before_limit,omitempty"`
	AfterLimit  int  `json:"after_limit,omitempty"`
	IncludeProfile bool `json:"include_profile,omitempty"`
}

type Groupings struct {
	GroupBy []Group `json:"group_by,omitempty"`
}

type Group struct {
	Key string `json:"key,omitempty"`
}

type SearchResponse struct {
	SearchCategories SearchCategoriesResult `json:"search_categories"`
}

type SearchCategoriesResult struct {
	RoomEvents *RoomEventsResult `json:"room_events,omitempty"`
}

type RoomEventsResult struct {
	Count     int                `json:"count"`
	Results   []SearchResult     `json:"results"`
	Highlights []string          `json:"highlights,omitempty"`
	State     map[string][]EventDTO `json:"state,omitempty"`
	Groups    map[string]map[string]SearchGroup `json:"groups,omitempty"`
	NextBatch string             `json:"next_batch,omitempty"`
}

type SearchResult struct {
	Rank   float64  `json:"rank"`
	Result EventDTO `json:"result"`
	Context *EventContextResponse `json:"context,omitempty"`
}

type SearchGroup struct {
	Order           int    `json:"order,omitempty"`
	NextBatch       string `json:"next_batch,omitempty"`
	Results         []SearchGroupResult `json:"results,omitempty"`
}

type SearchGroupResult struct {
	Rank   float64  `json:"rank"`
	Result EventDTO `json:"result"`
}

// ---------------------------------------------------------------------------
// Knock
// ---------------------------------------------------------------------------

type KnockRequest struct {
	Reason string `json:"reason,omitempty"`
}

type KnockResponse struct {
	RoomID string `json:"room_id"`
}

// ---------------------------------------------------------------------------
// TURN Server
// ---------------------------------------------------------------------------

type TurnServerResponse struct {
	URIs       []string `json:"uris"`
	Username   string   `json:"username"`
	Password   string   `json:"password"`
	TTL        int      `json:"ttl"`
}

// ---------------------------------------------------------------------------
// OpenID
// ---------------------------------------------------------------------------

type OpenIDRequestTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	MatrixServerName string `json:"matrix_server_name"`
	ExpiresIn        int64  `json:"expires_in"`
}

// ---------------------------------------------------------------------------
// Room upgrade
// ---------------------------------------------------------------------------

type RoomUpgradeRequest struct {
	NewVersion string `json:"new_version"`
}

type RoomUpgradeResponse struct {
	ReplacementRoom string `json:"replacement_room"`
}

// ---------------------------------------------------------------------------
// Read markers
// ---------------------------------------------------------------------------

type ReadMarkersRequest struct {
	FullyRead   string `json:"m.fully_read,omitempty"`
	Read        string `json:"m.read,omitempty"`
	ReadPrivate string `json:"m.read.private,omitempty"`
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

type NotificationsResponse struct {
	NextToken string                `json:"next_token,omitempty"`
	Notifications []NotificationDTO `json:"notifications"`
}

type NotificationDTO struct {
	Actions    []interface{} `json:"actions"`
	Event      EventDTO      `json:"event"`
	ProfileTag string        `json:"profile_tag,omitempty"`
	Read       bool          `json:"read"`
	RoomID     string        `json:"room_id"`
	TS         int64         `json:"ts"`
}

// ---------------------------------------------------------------------------
// Pushers
// ---------------------------------------------------------------------------

type PusherRequest struct {
	PushKey           string                 `json:"pushkey"`
	AppID             string                 `json:"app_id"`
	AppDisplayName    string                 `json:"app_display_name"`
	DeviceDisplayName string                 `json:"device_display_name"`
	Lang              string                 `json:"lang"`
	Kind              string                 `json:"kind"`
	Data              map[string]interface{} `json:"data,omitempty"`
	ProfileTag        string                 `json:"profile_tag,omitempty"`
	Append            bool                   `json:"append,omitempty"`
}

type PushersResponse struct {
	Pushers []PusherDTO `json:"pushers"`
}

type PusherDTO struct {
	PushKey           string                 `json:"pushkey"`
	AppID             string                 `json:"app_id"`
	AppDisplayName    string                 `json:"app_display_name"`
	DeviceDisplayName string                 `json:"device_display_name"`
	Lang              string                 `json:"lang"`
	Kind              string                 `json:"kind"`
	Data              map[string]interface{} `json:"data,omitempty"`
	ProfileTag        string                 `json:"profile_tag,omitempty"`
	Enabled           bool                   `json:"enabled"`
}

// ---------------------------------------------------------------------------
// Push Rules
// ---------------------------------------------------------------------------

type PushRulesResponse struct {
	Global PushRuleSet `json:"global"`
}

type PushRuleSet struct {
	Content   []PushRuleDTO `json:"content,omitempty"`
	Override  []PushRuleDTO `json:"override,omitempty"`
	Room      []PushRuleDTO `json:"room,omitempty"`
	Sender    []PushRuleDTO `json:"sender,omitempty"`
	Underride []PushRuleDTO `json:"underride,omitempty"`
}

type PushRuleDTO struct {
	RuleID     string        `json:"rule_id"`
	Actions    []interface{} `json:"actions"`
	Default    bool          `json:"default"`
	Enabled    bool          `json:"enabled"`
	Conditions []interface{} `json:"conditions,omitempty"`
	Pattern    string        `json:"pattern,omitempty"`
}

type PushRuleEnableRequest struct {
	Enabled bool `json:"enabled"`
}

type PushRuleActionsRequest struct {
	Actions []interface{} `json:"actions"`
}

// ---------------------------------------------------------------------------
// Third-party
// ---------------------------------------------------------------------------

type ThirdPartyProtocolsResponse map[string]ThirdPartyProtocol

type ThirdPartyProtocol struct {
	UserFields     []string          `json:"user_fields"`
	LocationFields []string          `json:"location_fields"`
	Icon           string            `json:"icon"`
	FieldTypes     map[string]FieldType `json:"field_types"`
	Instances      []ProtocolInstance `json:"instances"`
}

type FieldType struct {
	Regexp     string `json:"regexp"`
	Placeholder string `json:"placeholder"`
}

type ProtocolInstance struct {
	NetworkID string `json:"network_id"`
	Fields    map[string]interface{} `json:"fields"`
}

type ThirdPartyLocationRequest struct {
	Alias    string `json:"alias"`
	Language string `json:"language"`
}

type ThirdPartyUserRequest struct {
	UserID   string `json:"userid"`
	Language string `json:"language"`
}

type ThirdPartyLocationResponse []ThirdPartyLocation

type ThirdPartyLocation struct {
	Alias    string            `json:"alias"`
	Protocol string            `json:"protocol"`
	Fields   map[string]interface{} `json:"fields"`
}

type ThirdPartyUserResponse []ThirdPartyUser

type ThirdPartyUser struct {
	UserID   string            `json:"userid"`
	Protocol string            `json:"protocol"`
	Fields   map[string]interface{} `json:"fields"`
}

// ---------------------------------------------------------------------------
// Server Notices
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

type ReportRequest struct {
	Score  int    `json:"score"`
	Reason string `json:"reason"`
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
