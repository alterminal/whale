// Package user provides user management logic: registration, login,
// profile updates, and password changes.
package user

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"whale/storage"
)

// Service wraps the database and server configuration for user operations.
type Service struct {
	DB         *gorm.DB
	ServerName string // e.g., "matrix.example.com"
}

// RegisterParams holds the validated registration request.
type RegisterParams struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DeviceID    string `json:"device_id,omitempty"`
	InitialDisplayName string `json:"initial_device_display_name,omitempty"`
	InhibitLogin bool  `json:"inhibit_login"`
	Admin       bool   `json:"admin,omitempty"` // only allowed if requester is admin
}

// LoginParams holds validated login credentials.
type LoginParams struct {
	Type          string `json:"type"` // "m.login.password", "m.login.token"
	User          string `json:"user,omitempty"`          // deprecated; prefer Identifier
	Identifier    *UserIdentifier `json:"identifier,omitempty"`
	Password      string `json:"password,omitempty"`
	Token         string `json:"token,omitempty"`
	DeviceID      string `json:"device_id,omitempty"`
	InitialDisplayName string `json:"initial_device_display_name,omitempty"`
}

// UserIdentifier is used in the login identifier object.
type UserIdentifier struct {
	Type string `json:"type"` // "m.id.user", "m.id.thirdparty", "m.id.phone"
	User string `json:"user,omitempty"`
}

// LoginResult is the successful login response payload.
type LoginResult struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	DeviceID    string `json:"device_id"`
	HomeServer  string `json:"home_server,omitempty"` // deprecated but some clients expect it
	WellKnown   *WellKnownResponse `json:"well_known,omitempty"`
}

// WellKnownResponse is embedded in login responses for client discovery.
type WellKnownResponse struct {
	HomeServer struct {
		BaseURL string `json:"base_url"`
	} `json:"m.homeserver"`
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// Register creates a new Matrix user account. It validates that the
// localpart is available, hashes the password with bcrypt, and persists
// the user. If InhibitLogin is false, an access token is generated
// immediately (standard Matrix registration flow).
//
// Returns the full Matrix user ID, device ID, and access token on success.
func (s *Service) Register(params RegisterParams) (*LoginResult, error) {
	// Validate localpart
	localpart := strings.ToLower(params.Username)
	if localpart == "" {
		return nil, errors.New("username is required")
	}
	if err := validateLocalpart(localpart); err != nil {
		return nil, err
	}

	userID := fmt.Sprintf("@%s:%s", localpart, s.ServerName)

	// Check uniqueness
	var existing storage.User
	err := s.DB.Where("user_id = ?", userID).First(&existing).Error
	if err == nil {
		return nil, fmt.Errorf("user %s already exists", userID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Hash password (always bcrypt for now)
	hash, err := bcrypt.GenerateFromPassword([]byte(params.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := storage.User{
		UserID:         userID,
		PasswordHash:   string(hash),
		PasswordScheme: "bcrypt",
		IsAdmin:        params.Admin,
	}
	if err := s.DB.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// If inhibit_login, don't issue a token — return only user_id
	if params.InhibitLogin {
		return &LoginResult{
			UserID:     userID,
			HomeServer: s.ServerName,
		}, nil
	}

	// Generate device + access token
	return s.issueLogin(userID, params.DeviceID, params.InitialDisplayName)
}

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

// Login authenticates a user and returns an access token.
func (s *Service) Login(params LoginParams) (*LoginResult, error) {
	// Resolve the user identifier to a full MXID
	userID, err := s.resolveUser(params.User, params.Identifier)
	if err != nil {
		return nil, err
	}

	// Check user exists and is active
	var user storage.User
	if err := s.DB.Where("user_id = ? AND deactivated = false", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid username or password")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Authenticate
	switch params.Type {
	case "m.login.password", "":
		if params.Password == "" {
			return nil, errors.New("password is required for m.login.password")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(params.Password)); err != nil {
			return nil, errors.New("invalid username or password")
		}
	case "m.login.token":
		return nil, errors.New("token login not yet supported")
	default:
		return nil, fmt.Errorf("unsupported login type: %s", params.Type)
	}

	return s.issueLogin(userID, params.DeviceID, params.InitialDisplayName)
}

// ---------------------------------------------------------------------------
// Token management
// ---------------------------------------------------------------------------

// issueLogin creates a device (if a device_id was provided) and an access
// token, then returns the standard login result.
func (s *Service) issueLogin(userID, deviceID, displayName string) (*LoginResult, error) {
	// Generate device if needed
	if deviceID == "" {
		deviceID = generateDeviceID()
	}

	// Upsert device
	device := storage.Device{
		UserID:      userID,
		DeviceID:    deviceID,
		DisplayName: displayName,
	}
	if err := s.DB.Where("user_id = ? AND device_id = ?", userID, deviceID).
		Assign(storage.Device{DisplayName: displayName}).
		FirstOrCreate(&device).Error; err != nil {
		return nil, fmt.Errorf("failed to upsert device: %w", err)
	}

	// Generate access token
	token := generateToken()
	accessToken := storage.AccessToken{
		UserID:   userID,
		DeviceID: deviceID,
		Token:    token,
	}
	if err := s.DB.Create(&accessToken).Error; err != nil {
		return nil, fmt.Errorf("failed to create access token: %w", err)
	}

	return &LoginResult{
		UserID:      userID,
		AccessToken: token,
		DeviceID:    deviceID,
		HomeServer:  s.ServerName,
	}, nil
}

// Logout revokes the given access token.
func (s *Service) Logout(token string) error {
	result := s.DB.Where("token = ?", token).Delete(&storage.AccessToken{})
	if result.Error != nil {
		return fmt.Errorf("failed to revoke token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("token not found")
	}
	return nil
}

// LogoutAll revokes all access tokens for a given user.
func (s *Service) LogoutAll(userID string) error {
	return s.DB.Where("user_id = ?", userID).Delete(&storage.AccessToken{}).Error
}

// ValidateToken looks up an access token and returns the associated user ID.
// Returns ("", false) if the token is invalid or expired.
func (s *Service) ValidateToken(token string) (userID string, ok bool) {
	var at storage.AccessToken
	err := s.DB.Where("token = ?", token).First(&at).Error
	if err != nil {
		return "", false
	}
	// Check expiry
	if at.ExpiresAt != nil && time.Now().After(*at.ExpiresAt) {
		s.DB.Delete(&at) // best-effort cleanup
		return "", false
	}
	return at.UserID, true
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

// Profile holds a user's display name and avatar URL.
type Profile struct {
	DisplayName string `json:"displayname,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// GetProfile returns the display name and avatar for a user.
func (s *Service) GetProfile(userID string) (*Profile, error) {
	var u storage.User
	if err := s.DB.Where("user_id = ?", userID).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &Profile{
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
	}, nil
}

// SetDisplayName updates the user's display name.
func (s *Service) SetDisplayName(userID, displayName string) error {
	return s.DB.Model(&storage.User{}).Where("user_id = ?", userID).
		Update("display_name", displayName).Error
}

// SetAvatarURL updates the user's avatar URL.
func (s *Service) SetAvatarURL(userID, avatarURL string) error {
	return s.DB.Model(&storage.User{}).Where("user_id = ?", userID).
		Update("avatar_url", avatarURL).Error
}

// ---------------------------------------------------------------------------
// Password & account management
// ---------------------------------------------------------------------------

// ChangePassword updates the user's password and optionally revokes other tokens.
func (s *Service) ChangePassword(userID, newPassword string, logoutDevices bool) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&storage.User{}).Where("user_id = ?", userID).
			Updates(map[string]interface{}{
				"password_hash":   string(hash),
				"password_scheme": "bcrypt",
			}).Error; err != nil {
			return err
		}

		if logoutDevices {
			return tx.Where("user_id = ?", userID).Delete(&storage.AccessToken{}).Error
		}
		return nil
	})
}

// DeactivateAccount marks the user account as deactivated.
func (s *Service) DeactivateAccount(userID string, erase bool) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&storage.User{}).Where("user_id = ?", userID).
			Update("deactivated", true).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&storage.AccessToken{}).Error; err != nil {
			return err
		}
		tx.Where("user_id = ?", userID).Delete(&storage.RefreshToken{})
		if erase {
			tx.Model(&storage.User{}).Where("user_id = ?", userID).
				Updates(map[string]interface{}{
					"display_name": "",
					"avatar_url":   "",
				})
		}
		return nil
	})
}

// RefreshAccessToken issues a new access token from a valid refresh token.
func (s *Service) RefreshAccessToken(refreshToken string) (*LoginResult, error) {
	var rt storage.RefreshToken
	if err := s.DB.Where("token = ? AND used = false", refreshToken).First(&rt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid or expired refresh token")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	if rt.ExpiresAt != nil && time.Now().After(*rt.ExpiresAt) {
		return nil, errors.New("refresh token expired")
	}

	var at storage.AccessToken
	if err := s.DB.Where("id = ?", rt.AccessTokenID).First(&at).Error; err != nil {
		return nil, fmt.Errorf("access token not found: %w", err)
	}

	// Mark refresh token as used
	s.DB.Model(&rt).Update("used", true)

	return s.issueLogin(at.UserID, at.DeviceID, "")
}

// ---------------------------------------------------------------------------
// Device management
// ---------------------------------------------------------------------------

// GetDevices returns all devices for a user.
func (s *Service) GetDevices(userID string) ([]storage.Device, error) {
	var devices []storage.Device
	err := s.DB.Where("user_id = ?", userID).Find(&devices).Error
	return devices, err
}

// GetDevice returns a single device.
func (s *Service) GetDevice(userID, deviceID string) (*storage.Device, error) {
	var device storage.Device
	if err := s.DB.Where("user_id = ? AND device_id = ?", userID, deviceID).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

// UpdateDevice updates a device's display name.
func (s *Service) UpdateDevice(userID, deviceID, displayName string) error {
	return s.DB.Model(&storage.Device{}).
		Where("user_id = ? AND device_id = ?", userID, deviceID).
		Update("display_name", displayName).Error
}

// DeleteDevice deletes a device and its associated tokens.
func (s *Service) DeleteDevice(userID, deviceID, currentToken string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND device_id = ?", userID, deviceID).
			Delete(&storage.AccessToken{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND device_id = ?", userID, deviceID).
			Delete(&storage.Device{}).Error
	})
}

// DeleteDevices deletes multiple devices and their associated tokens.
func (s *Service) DeleteDevices(userID string, deviceIDs []string, currentToken string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var currentAT storage.AccessToken
		tx.Where("token = ?", currentToken).First(&currentAT)

		for _, deviceID := range deviceIDs {
			if currentAT.DeviceID == deviceID {
				continue // Don't delete own device
			}
			tx.Where("user_id = ? AND device_id = ?", userID, deviceID).
				Delete(&storage.AccessToken{})
			tx.Where("user_id = ? AND device_id = ?", userID, deviceID).
				Delete(&storage.Device{})
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveUser determines the full MXID from login parameters.
func (s *Service) resolveUser(user string, identifier *UserIdentifier) (string, error) {
	var localpart, domain string

	if identifier != nil {
		switch identifier.Type {
		case "m.id.user":
			parts := strings.SplitN(strings.TrimPrefix(identifier.User, "@"), ":", 2)
			if len(parts) != 2 {
				return "", fmt.Errorf("invalid user identifier: %s", identifier.User)
			}
			localpart, domain = parts[0], parts[1]
		default:
			return "", fmt.Errorf("unsupported identifier type: %s", identifier.Type)
		}
	} else if user != "" {
		parts := strings.SplitN(strings.TrimPrefix(user, "@"), ":", 2)
		if len(parts) == 2 {
			localpart, domain = parts[0], parts[1]
		} else {
			localpart = user
			domain = s.ServerName
		}
	} else {
		return "", errors.New("identifier or user is required")
	}

	if domain != s.ServerName {
		return "", fmt.Errorf("cannot log in as user from domain %s", domain)
	}

	return fmt.Sprintf("@%s:%s", strings.ToLower(localpart), domain), nil
}

// validateLocalpart enforces Matrix localpart rules.
func validateLocalpart(localpart string) error {
	if len(localpart) < 1 || len(localpart) > 255 {
		return errors.New("username must be between 1 and 255 characters")
	}
	if localpart == "_" {
		return errors.New("username cannot be exactly '_'")
	}
	for _, r := range localpart {
		if r < 0x21 || r == 0x3A { // reject control chars and ':'
			return fmt.Errorf("username contains invalid character: %q", r)
		}
	}
	return nil
}

// generateToken creates a cryptographically random 64-char hex token.
func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// generateDeviceID creates a random 10-char device ID.
func generateDeviceID() string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
