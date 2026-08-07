package client

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"whale/storage"
)

// =========================================================================
// Account Data — Global
// =========================================================================

// GET /_matrix/client/v3/user/{userId}/account_data/{type}
func (h *Handler) GetAccountData(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	authUserID := GetUserID(c)
	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot read another user's account data")
	}

	dataType := Param(c, "type")

	var ad storage.AccountData
	err := h.DB.Where("user_id = ? AND data_type = ?", userID, dataType).First(&ad).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Account data not found")
		}
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, ad.Content)
}

// PUT /_matrix/client/v3/user/{userId}/account_data/{type}
func (h *Handler) PutAccountData(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	authUserID := GetUserID(c)
	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot set another user's account data")
	}

	dataType := Param(c, "type")

	var raw map[string]interface{}
	if err := BindJSON(c, &raw); err != nil {
		return err
	}

	ad := storage.AccountData{
		UserID:   userID,
		DataType: dataType,
		Content:  storage.JSONMap(raw),
	}

	if err := h.DB.Where("user_id = ? AND data_type = ?", userID, dataType).
		Assign(ad).FirstOrCreate(&ad).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// Account Data — Per-room
// =========================================================================

// GET /_matrix/client/v3/user/{userId}/rooms/{roomId}/account_data/{type}
func (h *Handler) GetRoomAccountData(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	roomID := Param(c, "roomId")
	authUserID := GetUserID(c)
	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot read another user's room account data")
	}

	dataType := Param(c, "type")

	var ad storage.RoomAccountData
	err := h.DB.Where("user_id = ? AND room_id = ? AND data_type = ?", userID, roomID, dataType).First(&ad).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Room account data not found")
		}
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, ad.Content)
}

// PUT /_matrix/client/v3/user/{userId}/rooms/{roomId}/account_data/{type}
func (h *Handler) PutRoomAccountData(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	roomID := Param(c, "roomId")
	authUserID := GetUserID(c)
	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot set another user's room account data")
	}

	dataType := Param(c, "type")

	var raw map[string]interface{}
	if err := BindJSON(c, &raw); err != nil {
		return err
	}

	ad := storage.RoomAccountData{
		UserID:   userID,
		RoomID:   roomID,
		DataType: dataType,
		Content:  storage.JSONMap(raw),
	}

	if err := h.DB.Where("user_id = ? AND room_id = ? AND data_type = ?", userID, roomID, dataType).
		Assign(ad).FirstOrCreate(&ad).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/account/password
// =========================================================================

func (h *Handler) ChangePassword(c echo.Context) error {
	userID := GetUserID(c)

	var req PasswordRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if req.NewPassword == "" {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "new_password is required")
	}

	if err := h.UserSvc.ChangePassword(userID, req.NewPassword, req.LogoutDevices); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/account/deactivate
// =========================================================================

func (h *Handler) DeactivateAccount(c echo.Context) error {
	userID := GetUserID(c)

	var req DeactivateRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if err := h.UserSvc.DeactivateAccount(userID, req.Erase); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// POST /_matrix/client/v3/refresh
// =========================================================================

func (h *Handler) RefreshToken(c echo.Context) error {
	var req RefreshRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if req.RefreshToken == "" {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "refresh_token is required")
	}

	result, err := h.UserSvc.RefreshAccessToken(req.RefreshToken)
	if err != nil {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, err.Error())
	}

	return c.JSON(http.StatusOK, RefreshResponse{
		AccessToken: result.AccessToken,
	})
}

// =========================================================================
// Device management
// =========================================================================

// GET /_matrix/client/v3/devices
func (h *Handler) GetDevices(c echo.Context) error {
	userID := GetUserID(c)

	devices, err := h.UserSvc.GetDevices(userID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	dtos := make([]DeviceDTO, 0, len(devices))
	for _, d := range devices {
		dtos = append(dtos, DeviceDTO{
			DeviceID:    d.DeviceID,
			DisplayName: d.DisplayName,
			LastSeenIP:  d.LastSeenIP,
			LastSeenTS:  d.LastSeenAt.UnixMilli(),
		})
	}

	return c.JSON(http.StatusOK, DevicesResponse{Devices: dtos})
}

// GET /_matrix/client/v3/devices/{deviceId}
func (h *Handler) GetDevice(c echo.Context) error {
	userID := GetUserID(c)
	deviceID := Param(c, "deviceId")

	device, err := h.UserSvc.GetDevice(userID, deviceID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, DeviceDTO{
		DeviceID:    device.DeviceID,
		DisplayName: device.DisplayName,
		LastSeenIP:  device.LastSeenIP,
		LastSeenTS:  device.LastSeenAt.UnixMilli(),
	})
}

// PUT /_matrix/client/v3/devices/{deviceId}
func (h *Handler) UpdateDevice(c echo.Context) error {
	userID := GetUserID(c)
	deviceID := Param(c, "deviceId")

	var req DeviceUpdateRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if err := h.UserSvc.UpdateDevice(userID, deviceID, req.DisplayName); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// DELETE /_matrix/client/v3/devices/{deviceId}
func (h *Handler) DeleteDevice(c echo.Context) error {
	userID := GetUserID(c)
	deviceID := Param(c, "deviceId")

	// Extract the current token to avoid deleting own device if not intended
	token, _ := c.Get("access_token").(string)

	if err := h.UserSvc.DeleteDevice(userID, deviceID, token); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// POST /_matrix/client/v3/delete_devices
func (h *Handler) DeleteDevices(c echo.Context) error {
	userID := GetUserID(c)
	token, _ := c.Get("access_token").(string)

	var req DeleteDevicesRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if err := h.UserSvc.DeleteDevices(userID, req.Devices, token); err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}
