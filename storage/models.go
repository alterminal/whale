// Package storage provides GORM ORM models for the Matrix protocol entities.
//
// These models map directly to the Matrix specification's data model,
// covering the Client-Server API, Server-Server (Federation) API, and
// internal homeserver state.
//
// Table naming follows the Matrix spec conventions with whale-specific
// extensions where needed for efficient querying and indexing.
package storage

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// JSON types for Matrix event content, state, and rich payloads
// ---------------------------------------------------------------------------

// JSONMap is a generic JSON object stored as jsonb in PostgreSQL or JSON text
// in SQLite. It implements sql.Scanner and driver.Valuer for GORM
// compatibility.
type JSONMap map[string]interface{}

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}

// JSONSlice is a generic JSON array stored as jsonb.
type JSONSlice []interface{}

func (j JSONSlice) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONSlice) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}

// UnsignedData holds federation unsigned data (age, transaction_id, etc.).
type UnsignedData struct {
	Age           int64  `json:"age,omitempty"`
	TransactionID string `json:"transaction_id,omitempty"`
	RedactedBy    string `json:"redacted_by,omitempty"`
	RedactedBecause *EventContent `json:"redacted_because,omitempty"`
}

// EventContent embeds the full content of a Matrix event.
// Both the top-level content and any redaction reason use this.
type EventContent struct {
	Body          string `json:"body,omitempty"`
	MsgType       string `json:"msgtype,omitempty"`
	Format        string `json:"format,omitempty"`
	FormattedBody string `json:"formatted_body,omitempty"`
	URL           string `json:"url,omitempty"`
	Info          JSONMap `json:"info,omitempty" gorm:"type:jsonb"`
	Name          string `json:"name,omitempty"`
	Topic         string `json:"topic,omitempty"`
	Membership    string `json:"membership,omitempty"`
	DisplayName   string `json:"displayname,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	JoinRule      string `json:"join_rule,omitempty"`
	HistoryVisibility string `json:"history_visibility,omitempty"`
	GuestAccess   string `json:"guest_access,omitempty"`
	Reason        string `json:"reason,omitempty"`
	// Extensible: additional keys can be accessed via the raw JSONMap.
	Raw JSONMap `json:"-" gorm:"-"`
}

func (e EventContent) Value() (driver.Value, error) {
	return json.Marshal(e)
}

func (e *EventContent) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, e)
}

// ===========================================================================
// User & Authentication models
// ===========================================================================

// User represents a local Matrix user account.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#user-account-data
//
// The user_id is a fully qualified Matrix ID (@localpart:domain) and serves
// as the primary external identifier. Password hashes use bcrypt (default)
// but the algorithm is stored alongside the hash for future rotation.
type User struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID         string    `gorm:"uniqueIndex;size:255;not null" json:"user_id"`
	PasswordHash   string    `gorm:"size:255;not null" json:"-"`
	PasswordScheme string    `gorm:"size:32;default:bcrypt" json:"-"` // bcrypt, argon2id, etc.
	DisplayName    string    `gorm:"size:255" json:"display_name,omitempty"`
	AvatarURL      string    `gorm:"size:512" json:"avatar_url,omitempty"`
	IsAdmin        bool      `gorm:"default:false" json:"is_admin"`
	Deactivated    bool      `gorm:"default:false" json:"deactivated"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Devices      []Device      `gorm:"foreignKey:UserID;references:UserID" json:"-"`
	AccessTokens []AccessToken `gorm:"foreignKey:UserID;references:UserID" json:"-"`
}

func (User) TableName() string {
	return "users"
}

// Device represents an end-user client device for E2EE.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#device-management
//
// Each device belongs to exactly one user and is identified by a device_id
// that is unique within that user's scope. Devices are required for E2EE
// key management (Olm/Megolm sessions, one-time keys).
type Device struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID       string    `gorm:"index:idx_user_device,unique;size:255;not null" json:"user_id"`
	DeviceID     string    `gorm:"index:idx_user_device,unique;size:64;not null" json:"device_id"`
	DisplayName  string    `gorm:"size:255" json:"display_name,omitempty"`
	LastSeenIP   string    `gorm:"size:64" json:"last_seen_ip,omitempty"`
	LastSeenAt   time.Time `gorm:"autoUpdateTime" json:"last_seen_at"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Device) TableName() string {
	return "devices"
}

// AccessToken authenticates a user's session for API calls.
//
// Access tokens are bearer tokens passed in the Authorization header or
// the `access_token` query parameter. Each token is scoped to a device
// and can be revoked individually.
type AccessToken struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string    `gorm:"index;size:255;not null" json:"user_id"`
	DeviceID  string    `gorm:"size:64" json:"device_id,omitempty"`
	Token     string    `gorm:"uniqueIndex;size:512;not null" json:"token"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// Association
	User User `gorm:"foreignKey:UserID;references:UserID" json:"-"`
}

func (AccessToken) TableName() string {
	return "access_tokens"
}

// RefreshToken supports Matrix's token refresh flow (MSC2918 / Matrix 1.3+).
type RefreshToken struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	AccessTokenID uint     `gorm:"index;not null" json:"access_token_id"`
	Token        string    `gorm:"uniqueIndex;size:512;not null" json:"token"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Used         bool      `gorm:"default:false" json:"used"` // one-time use
}

func (RefreshToken) TableName() string {
	return "refresh_tokens"
}

// ===========================================================================
// Room & membership models
// ===========================================================================

// Room represents a Matrix chat room.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#rooms
//
// Rooms are the core communication primitive in Matrix. Each room is a
// directed acyclic graph (DAG) of events with a globally unique room_id.
// The version field tracks the room version for event authorization rules.
type Room struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	RoomID         string    `gorm:"uniqueIndex;size:255;not null" json:"room_id"`
	Name           string    `gorm:"size:512" json:"name,omitempty"`
	Topic          string    `gorm:"size:2048" json:"topic,omitempty"`
	CanonicalAlias string    `gorm:"size:512" json:"canonical_alias,omitempty"`
	Creator        string    `gorm:"size:255" json:"creator"`
	RoomVersion    string    `gorm:"size:32;default:10" json:"room_version"` // default to Matrix v10
	IsPublic       bool      `gorm:"default:false" json:"is_public"`
	JoinRules      string    `gorm:"size:32;default:invite" json:"join_rules"`
	Encryption     string    `gorm:"size:64" json:"encryption,omitempty"` // algorithm (m.megolm.v1.aes-sha2)
	Federatable    bool      `gorm:"default:true" json:"federatable"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Associations
	Memberships []RoomMembership `gorm:"foreignKey:RoomID;references:RoomID" json:"-"`
	Aliases     []RoomAlias      `gorm:"foreignKey:RoomID;references:RoomID" json:"-"`
}

func (Room) TableName() string {
	return "rooms"
}

// RoomAlias maps a human-readable alias (#alias:domain) to a room_id.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#room-aliases
//
// Aliases provide human-friendly room identifiers. A single room may have
// multiple aliases, but each alias can only point to one room at a time.
type RoomAlias struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	Alias     string    `gorm:"uniqueIndex;size:512;not null" json:"alias"`
	RoomID    string    `gorm:"index;size:255;not null" json:"room_id"`
	Creator   string    `gorm:"size:255" json:"creator"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (RoomAlias) TableName() string {
	return "room_aliases"
}

// RoomMembership tracks a user's relationship to a room.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#room-membership
//
// This is the authoritative local state for room membership derived from
// m.room.member events. Membership values: "invite", "join", "leave", "ban",
// "knock". The sender field records who initiated the membership change
// (often the user themselves for joins/leaves, or a room admin for bans).
type RoomMembership struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	RoomID      string    `gorm:"index:idx_room_member,unique;size:255;not null" json:"room_id"`
	UserID      string    `gorm:"index:idx_room_member,unique;size:255;not null" json:"user_id"`
	Sender      string    `gorm:"size:255;not null" json:"sender"`
	Membership  string    `gorm:"size:16;not null;default:leave" json:"membership"` // invite, join, leave, ban, knock
	DisplayName string    `gorm:"size:255" json:"display_name,omitempty"`
	AvatarURL   string    `gorm:"size:512" json:"avatar_url,omitempty"`
	Reason      string    `gorm:"size:2048" json:"reason,omitempty"`
	EventID     string    `gorm:"size:255" json:"event_id"` // last membership event
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (RoomMembership) TableName() string {
	return "room_memberships"
}

// ===========================================================================
// Event models — the core DAG
// ===========================================================================

// Event is a single entry in a room's event DAG.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#events
// https://spec.matrix.org/latest/server-server-api/#room-event-graph
//
// Events are the fundamental unit in Matrix. Every room is composed of
// events forming a directed acyclic graph. Events are immutable once
// accepted; redactions are themselves events.
//
// State events are identified by (room_id, event_type, state_key) triples.
// Message events have an empty state_key.
type Event struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	EventID         string    `gorm:"uniqueIndex;size:255;not null" json:"event_id"`
	RoomID          string    `gorm:"index:idx_room_type_state;size:255;not null" json:"room_id"`
	Sender          string    `gorm:"index;size:255;not null" json:"sender"`
	EventType       string    `gorm:"index:idx_room_type_state;size:128;not null" json:"type"`
	StateKey        *string   `gorm:"index:idx_room_type_state;size:512" json:"state_key,omitempty"` // NULL for message events
	Content         EventContent `gorm:"type:jsonb;not null" json:"content"`
	Redacts         string    `gorm:"size:255" json:"redacts,omitempty"` // event_id being redacted
	OriginServerTS  int64     `gorm:"not null" json:"origin_server_ts"`
	Depth           int64     `gorm:"not null;default:0" json:"depth"`
	Unsigned        JSONMap   `gorm:"type:jsonb" json:"unsigned,omitempty"`
	AuthEventsHash  string    `gorm:"size:128" json:"-"` // SHA-256 of sorted auth event IDs (for dedup)
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"-"`

	// Federation metadata
	HasBeenSentToFederation bool `gorm:"default:false" json:"-"`

	// Associations
	Room             Room           `gorm:"foreignKey:RoomID;references:RoomID" json:"-"`
	PrevEventRefs    []PrevEventRef `gorm:"foreignKey:EventID;references:EventID" json:"-"`
	AuthEventRefs    []AuthEventRef `gorm:"foreignKey:EventID;references:EventID" json:"-"`
}

func (Event) TableName() string {
	return "events"
}

// EventLike accessors — allow the event to be converted to an API DTO
// without importing storage from the client package.

func (e Event) EvtEventID() string        { return e.EventID }
func (e Event) EvtRoomID() string         { return e.RoomID }
func (e Event) EvtSender() string         { return e.Sender }
func (e Event) EvtEventType() string       { return e.EventType }
func (e Event) EvtStateKey() *string      { return e.StateKey }
func (e Event) EvtContent() interface{}   { return e.Content }
func (e Event) EvtOriginServerTS() int64  { return e.OriginServerTS }
func (e Event) EvtUnsigned() interface{}  { return e.Unsigned }
func (e Event) EvtRedacts() string        { return e.Redacts }

// PrevEventRef records the parent events in the DAG for a given event.
//
// Each event declares which events it immediately follows. These create
// the partial ordering that defines the room's event graph. The is_gap
// flag indicates whether we are missing events between this and its prev
// (relevant for federation backfill).
type PrevEventRef struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	EventID     string `gorm:"index:idx_prev,unique;size:255;not null" json:"event_id"`
	PrevEventID string `gorm:"index:idx_prev,unique;size:255;not null" json:"prev_event_id"`
	RoomID      string `gorm:"index;size:255;not null" json:"room_id"`
	IsGap       bool   `gorm:"default:false" json:"is_gap"`
}

func (PrevEventRef) TableName() string {
	return "prev_event_refs"
}

// AuthEventRef records which events authorize a given event.
//
// Auth events are the subset of state events that govern the validity
// of a new event according to the room's authorization rules (e.g.,
// m.room.create, m.room.power_levels, m.room.member of the sender).
type AuthEventRef struct {
	ID            uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	EventID       string `gorm:"index:idx_auth,unique;size:255;not null" json:"event_id"`
	AuthEventID   string `gorm:"index:idx_auth,unique;size:255;not null" json:"auth_event_id"`
	RoomID        string `gorm:"index;size:255;not null" json:"room_id"`
}

func (AuthEventRef) TableName() string {
	return "auth_event_refs"
}

// ===========================================================================
// State resolution helpers
// ===========================================================================

// CurrentRoomState caches the current value of every (event_type, state_key)
// tuple in a room. This denormalization allows O(1) lookups for state
// without walking the entire DAG — critical for /sync and /state requests.
//
// It is updated transactionally alongside each new event.
type CurrentRoomState struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	RoomID    string    `gorm:"index:idx_cur_state,priority:1;size:255;not null" json:"room_id"`
	EventType string    `gorm:"index:idx_cur_state,priority:2;size:128;not null" json:"event_type"`
	StateKey  string    `gorm:"index:idx_cur_state,priority:3;size:512;not null" json:"state_key"`
	EventID   string    `gorm:"index;size:255;not null" json:"event_id"`
	Content   JSONMap   `gorm:"type:jsonb;not null" json:"content"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CurrentRoomState) TableName() string {
	return "current_room_state"
}

// StateSnapshot captures the complete state tuple set resolved at a
// particular event. Used for historical state lookup and fast state
// resolution at merge points in the DAG.
type StateSnapshot struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	RoomID      string `gorm:"index:idx_snapshot;size:255;not null" json:"room_id"`
	EventID     string `gorm:"index:idx_snapshot;size:255;not null" json:"event_id"`
	EventType   string `gorm:"size:128;not null" json:"event_type"`
	StateKey    string `gorm:"size:512;not null" json:"state_key"`
	StateEventID string `gorm:"size:255;not null" json:"state_event_id"`
}

func (StateSnapshot) TableName() string {
	return "state_snapshots"
}

// ===========================================================================
// E2EE (End-to-End Encryption) models
// ===========================================================================

// OneTimeKey stores pre-uploaded one-time keys for the Olm protocol.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#key-management-api
//
// When a client wants to start an Olm session with a device, it claims
// one of these pre-published keys. Signed one-time keys (fallback keys)
// are tracked with the same model; the key_type field distinguishes them.
type OneTimeKey struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string    `gorm:"index:idx_otk_user_device;size:255;not null" json:"user_id"`
	DeviceID  string    `gorm:"index:idx_otk_user_device;size:64;not null" json:"device_id"`
	KeyID     string    `gorm:"size:64;not null" json:"key_id"` // key algorithm + key ID
	KeyType   string    `gorm:"size:32;default:signed_curve25519" json:"key_type"` // signed_curve25519 or fallback
	KeyJSON   JSONMap   `gorm:"type:jsonb;not null" json:"key_data"`
	Claimed   bool      `gorm:"default:false" json:"claimed"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (OneTimeKey) TableName() string {
	return "one_time_keys"
}

// DeviceKey stores long-term device identity keys (ed25519, curve25519).
//
// These are the public keys that identify a device in E2EE operations.
// They are published to the /keys/upload endpoint and queried via /keys/query.
type DeviceKey struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string    `gorm:"index:idx_dk_user_device_key;size:255;not null" json:"user_id"`
	DeviceID  string    `gorm:"index:idx_dk_user_device_key;size:64;not null" json:"device_id"`
	KeyID     string    `gorm:"index:idx_dk_user_device_key;size:64;not null" json:"key_id"` // e.g., "ed25519:DEVICEID"
	Algorithm string    `gorm:"size:32;not null" json:"algorithm"` // ed25519, curve25519
	PublicKey string    `gorm:"size:512;not null" json:"public_key"`
	Signatures JSONMap  `gorm:"type:jsonb" json:"signatures,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (DeviceKey) TableName() string {
	return "device_keys"
}

// ===========================================================================
// Federation / Server-Server API models
// ===========================================================================

// RemoteServer represents a peer Matrix homeserver known to this server.
//
// Used for federation: sending transactions, backfilling events, and
// making server-server API calls. Servers can be blacklisted if they
// misbehave.
type RemoteServer struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ServerName   string    `gorm:"uniqueIndex;size:255;not null" json:"server_name"`
	LastSeenAt   time.Time `gorm:"autoUpdateTime" json:"last_seen_at"`
	IsBlacklisted bool     `gorm:"default:false" json:"is_blacklisted"`
	BlacklistReason string  `gorm:"size:2048" json:"blacklist_reason,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (RemoteServer) TableName() string {
	return "remote_servers"
}

// ServerKey stores the verified signing keys of remote servers.
//
// Matrix spec: https://spec.matrix.org/latest/server-server-api/#server-keys
//
// Keys are fetched from /_matrix/key/v2/server and cached locally.
// The valid_until field enforces key rotation; keys are re-fetched
// before expiry. The key_id typically looks like "ed25519:abcd1234".
type ServerKey struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ServerName string    `gorm:"index;size:255;not null" json:"server_name"`
	KeyID      string    `gorm:"size:128;not null" json:"key_id"`
	Algorithm  string    `gorm:"size:32;not null;default:ed25519" json:"algorithm"`
	PublicKey  string    `gorm:"size:512;not null" json:"public_key"`
	ValidUntil time.Time `gorm:"not null" json:"valid_until"`
	Signatures JSONMap   `gorm:"type:jsonb" json:"signatures,omitempty"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (ServerKey) TableName() string {
	return "server_keys"
}

// FederationPDU stores incoming federation Protocol Data Units for
// processing before they are fully validated and converted to events.
//
// Used in the federation /send transaction endpoint. Events remain here
// until auth checks and state resolution complete.
type FederationPDU struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	TransactionID   string    `gorm:"index;size:255;not null" json:"transaction_id"`
	Origin          string    `gorm:"index;size:255;not null" json:"origin"`
	OriginServerTS  int64     `gorm:"not null" json:"origin_server_ts"`
	EventJSON       JSONMap   `gorm:"type:jsonb;not null" json:"event"`
	Processed       bool      `gorm:"default:false" json:"processed"`
	Error           string    `gorm:"size:2048" json:"error,omitempty"`
	ReceivedAt      time.Time `gorm:"autoCreateTime" json:"received_at"`
	ProcessedAt     *time.Time `json:"processed_at,omitempty"`
}

func (FederationPDU) TableName() string {
	return "federation_pdus"
}

// FederationEDU stores ephemeral data units (typing, presence, receipts)
// exchanged between servers but not persisted as DAG events.
type FederationEDU struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	TransactionID string    `gorm:"index;size:255;not null" json:"transaction_id"`
	Origin        string    `gorm:"index;size:255;not null" json:"origin"`
	EDUType       string    `gorm:"size:128;not null" json:"edu_type"`
	Content       JSONMap   `gorm:"type:jsonb;not null" json:"content"`
	ReceivedAt    time.Time `gorm:"autoCreateTime" json:"received_at"`
}

func (FederationEDU) TableName() string {
	return "federation_edus"
}

// ===========================================================================
// Sync & client state tracking
// ===========================================================================

// StreamToken tracks the user's position in the event stream for /sync.
//
// Each time a new event is persisted, a monotonically increasing stream
// position is assigned. The /sync endpoint returns a next_batch token
// that the client passes back to get only new events.
type StreamToken struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	Position int64  `gorm:"uniqueIndex;not null" json:"position"`
	EventID  string `gorm:"size:255" json:"event_id,omitempty"`
}

func (StreamToken) TableName() string {
	return "stream_tokens"
}

// AccountData stores per-user private key-value data synchronized across
// the user's devices (e.g., m.direct, m.push_rules, fully_read markers).
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#account-data
type AccountData struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string    `gorm:"index:idx_acc_data;size:255;not null" json:"user_id"`
	DataType  string    `gorm:"index:idx_acc_data;size:128;not null" json:"type"`
	Content   JSONMap   `gorm:"type:jsonb;not null" json:"content"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AccountData) TableName() string {
	return "account_data"
}

// RoomAccountData stores per-user, per-room private data (e.g., read
// markers, room-specific notification settings).
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#room-account-data
type RoomAccountData struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string    `gorm:"index:idx_room_acc_data;size:255;not null" json:"user_id"`
	RoomID    string    `gorm:"index:idx_room_acc_data;size:255;not null" json:"room_id"`
	DataType  string    `gorm:"index:idx_room_acc_data;size:128;not null" json:"type"`
	Content   JSONMap   `gorm:"type:jsonb;not null" json:"content"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (RoomAccountData) TableName() string {
	return "room_account_data"
}

// ===========================================================================
// Presence & typing indicators (ephemeral)
// ===========================================================================

// Presence tracks a user's online status for display to other users.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#presence
//
// Presence is ephemeral and not part of the event DAG. Status values
// are: "online", "offline", "unavailable". This table is updated via
// the /presence endpoint and polled by other users' clients.
type Presence struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID          string    `gorm:"uniqueIndex;size:255;not null" json:"user_id"`
	Presence        string    `gorm:"size:16;not null;default:offline" json:"presence"` // online, offline, unavailable
	StatusMessage   string    `gorm:"size:512" json:"status_msg,omitempty"`
	CurrentlyActive bool      `gorm:"default:false" json:"currently_active"`
	LastActiveAt    int64     `gorm:"not null" json:"last_active_ago"` // ms since last activity
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Presence) TableName() string {
	return "presence"
}

// Receipt tracks the last-read position per user per room (read receipts).
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#receipts
//
// Each receipt records the event_id a user has read up to, keyed by
// (user_id, room_id, receipt_type). The most common type is "m.read".
type Receipt struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID      string    `gorm:"index:idx_receipt;size:255;not null" json:"user_id"`
	RoomID      string    `gorm:"index:idx_receipt;size:255;not null" json:"room_id"`
	ReceiptType string    `gorm:"index:idx_receipt;size:32;not null;default:m.read" json:"receipt_type"`
	EventID     string    `gorm:"size:255;not null" json:"event_id"`
	ThreadID    string    `gorm:"size:255" json:"thread_id,omitempty"` // for threaded reads
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Receipt) TableName() string {
	return "receipts"
}

// ===========================================================================
// Filter models
// ===========================================================================

// Filter stores user-defined event filters for the /sync API.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#filtering
//
// Clients upload filters once and reference them by ID in subsequent
// /sync requests to reduce bandwidth. Filters are scoped to the creating
// user and can specify room, event type, sender, and timing constraints.
type Filter struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string    `gorm:"index;size:255;not null" json:"user_id"`
	FilterID  string    `gorm:"uniqueIndex;size:64;not null" json:"filter_id"`
	FilterDef JSONMap   `gorm:"type:jsonb;not null" json:"filter"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Filter) TableName() string {
	return "filters"
}

// ===========================================================================
// Application Service models
// ===========================================================================

// AppService represents a registered Application Service.
//
// Matrix spec: https://spec.matrix.org/latest/application-service-api/
//
// Application Services are privileged processes that can manage users
// and rooms across an entire homeserver. Each AS is identified by a
// token and has a set of user/alias regex namespaces it controls.
type AppService struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	Token     string    `gorm:"uniqueIndex;size:512;not null" json:"-"`
	URL       string    `gorm:"size:512;not null" json:"url"`
	SenderLocalpart string `gorm:"size:255;not null" json:"sender_localpart"`
	Namespaces JSONMap   `gorm:"type:jsonb;not null" json:"namespaces"` // users/rooms/aliases patterns
	RateLimited bool     `gorm:"default:true" json:"rate_limited"`
	Enabled    bool      `gorm:"default:true" json:"enabled"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (AppService) TableName() string {
	return "app_services"
}

// ===========================================================================
// Third-party invite / identity server models
// ===========================================================================

// ThirdPartyInvite stores pending invitations sent via identity servers
// (e.g., email invitations to non-Matrix users).
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#third-party-invites
type ThirdPartyInvite struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	RoomID      string    `gorm:"index;size:255;not null" json:"room_id"`
	Token       string    `gorm:"uniqueIndex;size:255;not null" json:"token"`
	Medium      string    `gorm:"size:32;not null" json:"medium"` // "email", "msisdn"
	Address     string    `gorm:"size:255;not null" json:"address"`
	Sender      string    `gorm:"size:255;not null" json:"sender"`
	DisplayName string    `gorm:"size:255" json:"display_name"`
	PublicKeys  JSONSlice `gorm:"type:jsonb" json:"public_keys,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (ThirdPartyInvite) TableName() string {
	return "third_party_invites"
}

// ===========================================================================
// Media & upload models
// ===========================================================================

// MediaRecord tracks uploaded media files.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#content-repository
//
// Media is identified by a server-generated MXC URI (mxc://server/media_id).
// The metadata includes MIME type, size, and optionally dimensions.
type MediaRecord struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	MediaID     string    `gorm:"uniqueIndex;size:255;not null" json:"media_id"`
	Origin      string    `gorm:"size:255;not null" json:"origin"` // local server_name or remote
	ContentType string    `gorm:"size:128;not null" json:"content_type"`
	SizeBytes   int64     `gorm:"not null" json:"size_bytes"`
	Width       int       `gorm:"default:0" json:"width,omitempty"`
	Height      int       `gorm:"default:0" json:"height,omitempty"`
	Uploader    string    `gorm:"size:255" json:"uploader"`
	FilePath    string    `gorm:"size:1024;not null" json:"-"` // local filesystem path
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (MediaRecord) TableName() string {
	return "media_records"
}

// ===========================================================================
// Push notification models
// ===========================================================================

// PushRule stores user-configurable push notification rules.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#push-rules
type PushRule struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID     string    `gorm:"index;size:255;not null" json:"user_id"`
	RuleID     string    `gorm:"size:255;not null" json:"rule_id"`
	RuleKind   string    `gorm:"size:32;not null" json:"kind"` // override, content, underride
	Actions    JSONSlice `gorm:"type:jsonb;not null" json:"actions"`
	Conditions JSONSlice `gorm:"type:jsonb" json:"conditions,omitempty"`
	Pattern    string    `gorm:"size:512" json:"pattern,omitempty"`
	Enabled    bool      `gorm:"default:true" json:"enabled"`
	DefaultRule bool     `gorm:"default:false" json:"default"`
	Priority   int       `gorm:"not null;default:0" json:"priority"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (PushRule) TableName() string {
	return "push_rules"
}

// Pusher stores push gateway registrations for out-of-app notifications.
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#pushers
type Pusher struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID            string    `gorm:"index;size:255;not null" json:"user_id"`
	PushKey           string    `gorm:"size:512;not null" json:"pushkey"`
	AppID             string    `gorm:"size:255;not null" json:"app_id"`
	AppDisplayName    string    `gorm:"size:255" json:"app_display_name"`
	DeviceDisplayName string    `gorm:"size:255" json:"device_display_name"`
	Lang              string    `gorm:"size:16;default:en" json:"lang"`
	Kind              string    `gorm:"size:32;not null;default:http" json:"kind"` // http, email
	Data              JSONMap   `gorm:"type:jsonb" json:"data,omitempty"` // pusher-specific payload
	Enabled           bool      `gorm:"default:true" json:"enabled"`
	CreatedAt         time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Pusher) TableName() string {
	return "pushers"
}

// ===========================================================================
// Rate limiting & admin
// ===========================================================================

// RateLimit tracks per-user API rate limiting counters.
type RateLimit struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	Key       string    `gorm:"index;size:255;not null" json:"key"` // user_id or IP
	Endpoint  string    `gorm:"size:255;not null" json:"endpoint"`
	Count     int64     `gorm:"not null;default:0" json:"count"`
	WindowStart time.Time `gorm:"not null" json:"window_start"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (RateLimit) TableName() string {
	return "rate_limits"
}

// ===========================================================================
// Room tags
// ===========================================================================

// RoomTag stores user-defined tags on rooms (e.g., "m.favourite", "m.lowpriority").
//
// Matrix spec: https://spec.matrix.org/latest/client-server-api/#room-tags
type RoomTag struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string    `gorm:"index:idx_room_tag,unique;size:255;not null" json:"user_id"`
	RoomID    string    `gorm:"index:idx_room_tag,unique;size:255;not null" json:"room_id"`
	Tag       string    `gorm:"index:idx_room_tag,unique;size:128;not null" json:"tag"`
	Order     JSONMap   `gorm:"type:jsonb" json:"order,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (RoomTag) TableName() string {
	return "room_tags"
}

// ===========================================================================
// Auto-migration helper
// ===========================================================================

// AutoMigrate runs GORM AutoMigrate on all model tables.
//
// In production, migrations should be managed explicitly with versioned
// SQL files. This is provided as a convenience for development and for
// bootstrapping new deployments.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&Device{},
		&AccessToken{},
		&RefreshToken{},
		&Room{},
		&RoomAlias{},
		&RoomMembership{},
		&Event{},
		&PrevEventRef{},
		&AuthEventRef{},
		&CurrentRoomState{},
		&StateSnapshot{},
		&OneTimeKey{},
		&DeviceKey{},
		&RemoteServer{},
		&ServerKey{},
		&FederationPDU{},
		&FederationEDU{},
		&StreamToken{},
		&AccountData{},
		&RoomAccountData{},
		&Presence{},
		&Receipt{},
		&Filter{},
		&AppService{},
		&ThirdPartyInvite{},
		&MediaRecord{},
		&PushRule{},
		&Pusher{},
		&RateLimit{},
		&RoomTag{},
	)
}
