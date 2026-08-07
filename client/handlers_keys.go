package client

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"whale/storage"
)

// =========================================================================
// POST /_matrix/client/v3/keys/upload
// =========================================================================

func (h *Handler) UploadKeys(c echo.Context) error {
	userID := GetUserID(c)

	var req KeyUploadRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrNotJSON, "Invalid JSON")
	}

	// Store device keys
	if req.DeviceKeys != nil {
		for keyID, pubKey := range req.DeviceKeys.Keys {
			dk := storage.DeviceKey{
				UserID:    userID,
				DeviceID:  req.DeviceKeys.DeviceID,
				KeyID:     keyID,
				Algorithm: algorithmFromKeyID(keyID),
				PublicKey: pubKey,
				Signatures: storage.JSONMap{},
			}
			// Copy signatures
			if req.DeviceKeys.Signatures != nil {
				sigMap := make(map[string]interface{})
				for k, v := range req.DeviceKeys.Signatures {
					inner := make(map[string]interface{})
					for ik, iv := range v {
						inner[ik] = iv
					}
					sigMap[k] = inner
				}
				dk.Signatures = sigMap
			}
			h.DB.Where("user_id = ? AND device_id = ? AND key_id = ?", userID, req.DeviceKeys.DeviceID, keyID).
				Assign(dk).FirstOrCreate(&dk)
		}
	}

	// Store one-time keys
	if req.OneTimeKeys != nil {
		for keyID, keyData := range req.OneTimeKeys {
			otk := storage.OneTimeKey{
				UserID:   userID,
				DeviceID: req.DeviceKeys.DeviceID,
				KeyID:    keyID,
				KeyType:  "signed_curve25519",
				KeyJSON:  toJSONMap(keyData),
			}
			h.DB.Create(&otk)
		}
	}

	// Store fallback keys
	if req.FallbackKeys != nil {
		for keyID, keyData := range req.FallbackKeys {
			otk := storage.OneTimeKey{
				UserID:   userID,
				DeviceID: req.DeviceKeys.DeviceID,
				KeyID:    keyID,
				KeyType:  "fallback",
				KeyJSON:  toJSONMap(keyData),
			}
			h.DB.Create(&otk)
		}
	}

	// Count remaining one-time keys
	var signedCount, fallbackCount int64
	h.DB.Model(&storage.OneTimeKey{}).
		Where("user_id = ? AND device_id = ? AND key_type = ? AND claimed = false", userID, req.DeviceKeys.DeviceID, "signed_curve25519").
		Count(&signedCount)
	h.DB.Model(&storage.OneTimeKey{}).
		Where("user_id = ? AND device_id = ? AND key_type = ? AND claimed = false", userID, req.DeviceKeys.DeviceID, "fallback").
		Count(&fallbackCount)

	return c.JSON(http.StatusOK, KeyUploadResponse{
		OneTimeKeyCounts: map[string]int{
			"signed_curve25519": int(signedCount),
			"fallback":          int(fallbackCount),
		},
	})
}

// =========================================================================
// POST /_matrix/client/v3/keys/query
// =========================================================================

func (h *Handler) QueryKeys(c echo.Context) error {
	var req KeyQueryRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrNotJSON, "Invalid JSON")
	}

	result := make(map[string]map[string]DeviceKeyDTO)
	failures := make(map[string]interface{})

	for queryUserID, deviceIDs := range req.DeviceKeys {
		var dks []storage.DeviceKey
		tx := h.DB.Where("user_id = ?", queryUserID)
		if len(deviceIDs) > 0 {
			tx = tx.Where("device_id IN ?", deviceIDs)
		}
		if err := tx.Find(&dks).Error; err != nil && err != gorm.ErrRecordNotFound {
			failures[queryUserID] = map[string]string{"error": err.Error()}
			continue
		}

		if len(dks) == 0 {
			failures[queryUserID] = map[string]string{"error": "No device keys found"}
			continue
		}

		deviceMap := make(map[string]DeviceKeyDTO)
		for _, dk := range dks {
			if _, exists := deviceMap[dk.DeviceID]; !exists {
				deviceMap[dk.DeviceID] = DeviceKeyDTO{
					UserID:     dk.UserID,
					DeviceID:   dk.DeviceID,
					Algorithms: []string{"m.olm.v1.curve25519-aes-sha2", "m.megolm.v1.aes-sha2"},
					Keys:       make(map[string]string),
					Signatures: make(map[string]map[string]string),
				}
			}
			dto := deviceMap[dk.DeviceID]
			dto.Keys[dk.KeyID] = dk.PublicKey
			// Convert signatures
			if dk.Signatures != nil {
				for userID, sigs := range dk.Signatures {
					if sigMap, ok := sigs.(map[string]interface{}); ok {
						dto.Signatures[userID] = make(map[string]string)
						for k, v := range sigMap {
							if sv, ok := v.(string); ok {
								dto.Signatures[userID][k] = sv
							}
						}
					}
				}
			}
			deviceMap[dk.DeviceID] = dto
		}
		result[queryUserID] = deviceMap
	}

	return c.JSON(http.StatusOK, KeyQueryResponse{
		DeviceKeys: result,
		Failures:   failures,
	})
}

// =========================================================================
// POST /_matrix/client/v3/keys/claim
// =========================================================================

func (h *Handler) ClaimKeys(c echo.Context) error {
	var req KeyClaimRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrNotJSON, "Invalid JSON")
	}

	result := make(map[string]map[string]interface{})
	failures := make(map[string]interface{})

	for claimUserID, deviceMap := range req.OneTimeKeys {
		for deviceID, algorithm := range deviceMap {
			var otk storage.OneTimeKey
			err := h.DB.Where("user_id = ? AND device_id = ? AND key_type = ? AND claimed = false",
				claimUserID, deviceID, "signed_curve25519").
				Order("id ASC").First(&otk).Error

			if err != nil {
				if err == gorm.ErrRecordNotFound {
					failures[claimUserID+":"+deviceID] = map[string]string{"error": "No keys available"}
				}
				continue
			}

			// Mark as claimed
			h.DB.Model(&otk).Update("claimed", true)

			// Add to result
			if result[claimUserID] == nil {
				result[claimUserID] = make(map[string]interface{})
			}
			result[claimUserID][deviceID] = map[string]interface{}{
				algorithm: otk.KeyJSON,
			}
		}
	}

	return c.JSON(http.StatusOK, KeyClaimResponse{
		OneTimeKeys: result,
		Failures:    failures,
	})
}

// helpers

func algorithmFromKeyID(keyID string) string {
	if len(keyID) >= 8 && keyID[:8] == "ed25519:" {
		return "ed25519"
	}
	if len(keyID) >= 12 && keyID[:12] == "curve25519:" {
		return "curve25519"
	}
	return keyID
}

func toJSONMap(v interface{}) storage.JSONMap {
	if m, ok := v.(map[string]interface{}); ok {
		return storage.JSONMap(m)
	}
	return storage.JSONMap{"value": v}
}

func generateFilterID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
