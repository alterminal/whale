package client

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"whale/room"
	"whale/storage"
)

// =========================================================================
// Search
// =========================================================================

// POST /_matrix/client/v3/search
func (h *Handler) Search(c echo.Context) error {
	userID := GetUserID(c)

	var req SearchRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if req.SearchCategories.RoomEvents == nil {
		return h.ErrorResponse(c, http.StatusBadRequest, ErrInvalidParam, "search_categories.room_events is required")
	}

	re := req.SearchCategories.RoomEvents
	limit := 20

	// Get user's joined rooms
	joinedRooms, err := h.RoomSvc.GetJoinedRooms(userID)
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	if len(joinedRooms) == 0 {
		return c.JSON(http.StatusOK, SearchResponse{
			SearchCategories: SearchCategoriesResult{
				RoomEvents: &RoomEventsResult{
					Count:   0,
					Results: []SearchResult{},
				},
			},
		})
	}

	// Search for the term across user's rooms
	searchTerm := "%" + strings.ToLower(re.SearchTerm) + "%"

	var events []storage.Event
	tx := h.DB.Where("room_id IN ? AND event_type = ?", joinedRooms, "m.room.message").
		Where("LOWER(content) LIKE ?", searchTerm).
		Order("origin_server_ts DESC").Limit(limit)

	if err := tx.Find(&events).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	results := make([]SearchResult, 0, len(events))
	for _, ev := range events {
		results = append(results, SearchResult{
			Rank:   1.0,
			Result: EventToDTO(ev),
		})
	}

	return c.JSON(http.StatusOK, SearchResponse{
		SearchCategories: SearchCategoriesResult{
			RoomEvents: &RoomEventsResult{
				Count:   len(results),
				Results: results,
			},
		},
	})
}

// =========================================================================
// Knock
// =========================================================================

// POST /_matrix/client/v3/knock/{roomIdOrAlias}
func (h *Handler) KnockRoom(c echo.Context) error {
	roomIDOrAlias := Param(c, "roomIdOrAlias")
	userID := GetUserID(c)

	var req KnockRequest
	BindJSON(c, &req)

	// Resolve alias if needed
	roomID := roomIDOrAlias
	if strings.HasPrefix(roomIDOrAlias, "#") {
		alias := roomIDOrAlias
		if !strings.Contains(alias, ":") {
			alias = alias + ":" + h.ServerName
		}
		var ra storage.RoomAlias
		if err := h.DB.Where("alias = ?", alias).First(&ra).Error; err != nil {
			return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Room alias not found")
		}
		roomID = ra.RoomID
	}

	// Check room exists
	var room storage.Room
	if err := h.DB.Where("room_id = ?", roomID).First(&room).Error; err != nil {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Room not found")
	}

	// Create knock membership
	eventID := generateFilterID()
	m := storage.RoomMembership{
		RoomID:     roomID,
		UserID:     userID,
		Sender:     userID,
		Membership: "knock",
		Reason:     req.Reason,
		EventID:    eventID,
	}

	if err := h.DB.Where("room_id = ? AND user_id = ?", roomID, userID).
		Assign(m).FirstOrCreate(&m).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, KnockResponse{RoomID: roomID})
}

// =========================================================================
// TURN Server
// =========================================================================

// GET /_matrix/client/v3/voip/turnServer
func (h *Handler) TurnServer(c echo.Context) error {
	// Return an empty response — TURN server requires external configuration
	return c.JSON(http.StatusOK, TurnServerResponse{
		URIs:     []string{},
		Username: "",
		Password: "",
		TTL:      3600,
	})
}

// =========================================================================
// OpenID
// =========================================================================

// POST /_matrix/client/v3/user/{userId}/openid/request_token
func (h *Handler) OpenIDToken(c echo.Context) error {
	userID := normalizeUserID(Param(c, "userId"), h.ServerName)
	authUserID := GetUserID(c)

	if authUserID != userID {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Cannot request OpenID token for another user")
	}

	// Generate a simple OpenID token
	token := generateFilterID() + generateFilterID()

	return c.JSON(http.StatusOK, OpenIDRequestTokenResponse{
		AccessToken:      token,
		TokenType:        "Bearer",
		MatrixServerName: h.ServerName,
		ExpiresIn:        3600,
	})
}

// =========================================================================
// Room upgrade
// =========================================================================

// POST /_matrix/client/v3/rooms/{roomId}/upgrade
func (h *Handler) UpgradeRoom(c echo.Context) error {
	roomID := Param(c, "roomId")
	userID := GetUserID(c)

	if !h.RoomSvc.IsMember(roomID, userID) {
		return h.ErrorResponse(c, http.StatusForbidden, ErrForbidden, "Not a member of this room")
	}

	var req RoomUpgradeRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	// Create a new room with the new version
	result, err := h.RoomSvc.CreateRoom(room.CreateRoomParams{
		Creator:     userID,
		RoomVersion: req.NewVersion,
		Preset:      "private_chat",
	})
	if err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, RoomUpgradeResponse{ReplacementRoom: result.RoomID})
}

// =========================================================================
// Pushers
// =========================================================================

// POST /_matrix/client/v3/pushers/set
func (h *Handler) SetPusher(c echo.Context) error {
	userID := GetUserID(c)

	var req PusherRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	data := storage.JSONMap{}
	if req.Data != nil {
		data = storage.JSONMap(req.Data)
	}

	pusher := storage.Pusher{
		UserID:            userID,
		PushKey:           req.PushKey,
		AppID:             req.AppID,
		AppDisplayName:    req.AppDisplayName,
		DeviceDisplayName: req.DeviceDisplayName,
		Lang:              req.Lang,
		Kind:              req.Kind,
		Data:              data,
		Enabled:           true,
	}

	if !req.Append {
		// Replace mode — delete existing then create
		h.DB.Where("user_id = ? AND push_key = ? AND app_id = ?", userID, req.PushKey, req.AppID).Delete(&storage.Pusher{})
	}

	if err := h.DB.Where("user_id = ? AND push_key = ? AND app_id = ?", userID, req.PushKey, req.AppID).
		Assign(pusher).FirstOrCreate(&pusher).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// GET /_matrix/client/v3/pushers
func (h *Handler) GetPushers(c echo.Context) error {
	userID := GetUserID(c)

	var pushers []storage.Pusher
	if err := h.DB.Where("user_id = ?", userID).Find(&pushers).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	result := make([]PusherDTO, 0, len(pushers))
	for _, p := range pushers {
		data := map[string]interface{}(p.Data)
		result = append(result, PusherDTO{
			PushKey:           p.PushKey,
			AppID:             p.AppID,
			AppDisplayName:    p.AppDisplayName,
			DeviceDisplayName: p.DeviceDisplayName,
			Lang:              p.Lang,
			Kind:              p.Kind,
			Data:              data,
			Enabled:           p.Enabled,
		})
	}

	return c.JSON(http.StatusOK, PushersResponse{Pushers: result})
}

// =========================================================================
// Push Rules
// =========================================================================

// GET /_matrix/client/v3/pushrules/
func (h *Handler) GetPushRules(c echo.Context) error {
	userID := GetUserID(c)

	var rules []storage.PushRule
	h.DB.Where("user_id = ?", userID).Find(&rules)

	response := PushRulesResponse{
		Global: PushRuleSet{
			Override:  []PushRuleDTO{},
			Content:   []PushRuleDTO{},
			Room:      []PushRuleDTO{},
			Sender:    []PushRuleDTO{},
			Underride: []PushRuleDTO{},
		},
	}

	for _, r := range rules {
		dto := PushRuleDTO{
			RuleID:     r.RuleID,
			Actions:    []interface{}(r.Actions),
			Default:    r.DefaultRule,
			Enabled:    r.Enabled,
			Conditions: []interface{}(r.Conditions),
			Pattern:    r.Pattern,
		}
		switch r.RuleKind {
		case "override":
			response.Global.Override = append(response.Global.Override, dto)
		case "content":
			response.Global.Content = append(response.Global.Content, dto)
		case "room":
			response.Global.Room = append(response.Global.Room, dto)
		case "sender":
			response.Global.Sender = append(response.Global.Sender, dto)
		case "underride":
			response.Global.Underride = append(response.Global.Underride, dto)
		}
	}

	return c.JSON(http.StatusOK, response)
}

// GET /_matrix/client/v3/pushrules/{scope}/{kind}/{ruleId}
func (h *Handler) GetPushRule(c echo.Context) error {
	userID := GetUserID(c)
	ruleID := Param(c, "ruleId")
	// scope := Param(c, "scope")  // always "global" for now
	// kind := Param(c, "kind")

	var rule storage.PushRule
	if err := h.DB.Where("user_id = ? AND rule_id = ?", userID, ruleID).First(&rule).Error; err != nil {
		return h.ErrorResponse(c, http.StatusNotFound, ErrNotFound, "Push rule not found")
	}

	return c.JSON(http.StatusOK, PushRuleDTO{
		RuleID:     rule.RuleID,
		Actions:    []interface{}(rule.Actions),
		Default:    rule.DefaultRule,
		Enabled:    rule.Enabled,
		Conditions: []interface{}(rule.Conditions),
		Pattern:    rule.Pattern,
	})
}

// PUT /_matrix/client/v3/pushrules/{scope}/{kind}/{ruleId}
func (h *Handler) PutPushRule(c echo.Context) error {
	userID := GetUserID(c)
	ruleID := Param(c, "ruleId")
	kind := Param(c, "kind")

	var req PushRuleDTO
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	actions := storage.JSONSlice{}
	for _, a := range req.Actions {
		actions = append(actions, a)
	}

	conditions := storage.JSONSlice{}
	for _, c := range req.Conditions {
		conditions = append(conditions, c)
	}

	rule := storage.PushRule{
		UserID:     userID,
		RuleID:     ruleID,
		RuleKind:   kind,
		Actions:    actions,
		Conditions: conditions,
		Pattern:    req.Pattern,
		Enabled:    req.Enabled,
	}

	if err := h.DB.Where("user_id = ? AND rule_id = ?", userID, ruleID).
		Assign(rule).FirstOrCreate(&rule).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// DELETE /_matrix/client/v3/pushrules/{scope}/{kind}/{ruleId}
func (h *Handler) DeletePushRule(c echo.Context) error {
	userID := GetUserID(c)
	ruleID := Param(c, "ruleId")

	if err := h.DB.Where("user_id = ? AND rule_id = ?", userID, ruleID).
		Delete(&storage.PushRule{}).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// PUT /_matrix/client/v3/pushrules/{scope}/{kind}/{ruleId}/enabled
func (h *Handler) SetPushRuleEnabled(c echo.Context) error {
	userID := GetUserID(c)
	ruleID := Param(c, "ruleId")

	var req PushRuleEnableRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	if err := h.DB.Model(&storage.PushRule{}).
		Where("user_id = ? AND rule_id = ?", userID, ruleID).
		Update("enabled", req.Enabled).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// PUT /_matrix/client/v3/pushrules/{scope}/{kind}/{ruleId}/actions
func (h *Handler) SetPushRuleActions(c echo.Context) error {
	userID := GetUserID(c)
	ruleID := Param(c, "ruleId")

	var req PushRuleActionsRequest
	if err := BindJSON(c, &req); err != nil {
		return err
	}

	actions := storage.JSONSlice{}
	for _, a := range req.Actions {
		actions = append(actions, a)
	}

	if err := h.DB.Model(&storage.PushRule{}).
		Where("user_id = ? AND rule_id = ?", userID, ruleID).
		Update("actions", actions).Error; err != nil {
		return h.ErrorResponse(c, http.StatusInternalServerError, ErrUnknown, err.Error())
	}

	return c.JSON(http.StatusOK, EmptyResponse{})
}

// =========================================================================
// Notifications
// =========================================================================

// GET /_matrix/client/v3/notifications
func (h *Handler) GetNotifications(c echo.Context) error {
	userID := GetUserID(c)
	limit := queryInt(c, "limit", 20)

	// Get recent messages from joined rooms
	joinedRooms, _ := h.RoomSvc.GetJoinedRooms(userID)
	if len(joinedRooms) == 0 {
		return c.JSON(http.StatusOK, NotificationsResponse{
			Notifications: []NotificationDTO{},
		})
	}

	var events []storage.Event
	h.DB.Where("room_id IN ? AND event_type = ?", joinedRooms, "m.room.message").
		Order("origin_server_ts DESC").Limit(limit).Find(&events)

	notifications := make([]NotificationDTO, 0, len(events))
	for _, ev := range events {
		notifications = append(notifications, NotificationDTO{
			Actions: []interface{}{"notify"},
			Event:   EventToDTO(ev),
			Read:    false,
			RoomID:  ev.RoomID,
			TS:      ev.OriginServerTS,
		})
	}

	return c.JSON(http.StatusOK, NotificationsResponse{
		Notifications: notifications,
	})
}

// =========================================================================
// Third-party protocols / location / user
// =========================================================================

// GET /_matrix/client/v3/thirdparty/protocols
func (h *Handler) GetThirdPartyProtocols(c echo.Context) error {
	return c.JSON(http.StatusOK, ThirdPartyProtocolsResponse{})
}

// GET /_matrix/client/v3/thirdparty/protocol/{protocol}
func (h *Handler) GetThirdPartyProtocol(c echo.Context) error {
	return c.JSON(http.StatusOK, ThirdPartyProtocol{})
}

// GET /_matrix/client/v3/thirdparty/location
func (h *Handler) GetThirdPartyLocation(c echo.Context) error {
	return c.JSON(http.StatusOK, ThirdPartyLocationResponse{})
}

// GET /_matrix/client/v3/thirdparty/user
func (h *Handler) GetThirdPartyUser(c echo.Context) error {
	return c.JSON(http.StatusOK, ThirdPartyUserResponse{})
}
