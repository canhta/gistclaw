package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
	"github.com/canhta/gistclaw/internal/authority"
)

type settingsResponse struct {
	Machine settingsMachineResponse `json:"machine"`
	Access  settingsAccessResponse  `json:"access"`
}

type settingsMachineResponse struct {
	StorageRoot           string  `json:"storage_root"`
	ApprovalMode          string  `json:"approval_mode"`
	ApprovalModeLabel     string  `json:"approval_mode_label"`
	HostAccessMode        string  `json:"host_access_mode"`
	HostAccessModeLabel   string  `json:"host_access_mode_label"`
	AdminToken            string  `json:"admin_token"`
	PerRunTokenBudget     string  `json:"per_run_token_budget"`
	DailyCostCapUSD       string  `json:"daily_cost_cap_usd"`
	RollingCostUSD        float64 `json:"rolling_cost_usd"`
	RollingCostLabel      string  `json:"rolling_cost_label"`
	TelegramToken         string  `json:"telegram_token"`
	WhatsAppPhoneNumberID string  `json:"whatsapp_phone_number_id"`
	WhatsAppAccessToken   string  `json:"whatsapp_access_token"`
	WhatsAppVerifyToken   string  `json:"whatsapp_verify_token"`
	ActiveProjectName     string  `json:"active_project_name"`
	ActiveProjectPath     string  `json:"active_project_path"`
	ActiveProjectSummary  string  `json:"active_project_summary"`
}

type settingsAccessResponse struct {
	PasswordConfigured bool                     `json:"password_configured"`
	CurrentDevice      *settingsDeviceResponse  `json:"current_device,omitempty"`
	OtherActiveDevices []settingsDeviceResponse `json:"other_active_devices"`
	BlockedDevices     []settingsDeviceResponse `json:"blocked_devices"`
}

type settingsDeviceResponse struct {
	ID               string `json:"id"`
	PrimaryLabel     string `json:"primary_label"`
	SecondaryLine    string `json:"secondary_line"`
	Current          bool   `json:"current"`
	Blocked          bool   `json:"blocked"`
	ActiveSessions   int    `json:"active_sessions"`
	DetailsIP        string `json:"details_ip"`
	DetailsUserAgent string `json:"details_user_agent"`
}

type settingsActionResponse struct {
	Notice    string            `json:"notice,omitempty"`
	LoggedOut bool              `json:"logged_out,omitempty"`
	Next      string            `json:"next,omitempty"`
	Settings  *settingsResponse `json:"settings,omitempty"`
}

type settingsUpdateRequest struct {
	ApprovalMode          *string `json:"approval_mode"`
	HostAccessMode        *string `json:"host_access_mode"`
	PerRunTokenBudget     *string `json:"per_run_token_budget"`
	DailyCostCapUSD       *string `json:"daily_cost_cap_usd"`
	TelegramBotToken      *string `json:"telegram_bot_token"`
	WhatsAppPhoneNumberID *string `json:"whatsapp_phone_number_id"`
	WhatsAppAccessToken   *string `json:"whatsapp_access_token"`
	WhatsAppVerifyToken   *string `json:"whatsapp_verify_token"`
}

type settingsPasswordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

type settingsDeviceMutationResult struct {
	Notice    string
	LoggedOut bool
	Next      string
}

type settingsMachineSnapshot struct {
	StorageRoot           string
	ApprovalMode          string
	HostAccessMode        string
	AdminToken            string
	PerRunTokenBudget     string
	DailyCostCapUSD       string
	RollingCostUSD        float64
	TelegramToken         string
	WhatsAppPhoneNumberID string
	WhatsAppAccessToken   string
	WhatsAppVerifyToken   string
}

func (s *Server) handleSettingsAPI(w http.ResponseWriter, r *http.Request) {
	resp, err := s.loadSettingsResponse(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSettingsUpdateAPI(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	var req settingsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}

	updates, err := s.settingsUpdatesFromRequest(req)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
		return
	}
	if err := s.rt.UpdateSettings(r.Context(), updates); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": settingsUpdateErrorMessage(err)})
		return
	}

	resp, err := s.loadSettingsResponse(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, settingsActionResponse{
		Notice:   "Machine settings updated.",
		Settings: &resp,
	})
}

func (s *Server) handleSettingsPasswordChangeAPI(w http.ResponseWriter, r *http.Request) {
	var req settingsPasswordChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}

	issued, err := s.changePasswordAndReauthenticate(r, req.CurrentPassword, req.NewPassword, req.ConfirmPassword)
	if err != nil {
		if errors.Is(err, errPasswordReauthenticationFailed) {
			clearAuthCookies(w, r)
			writeJSON(w, http.StatusOK, settingsActionResponse{
				Notice:    "Password updated. Sign in again to keep working.",
				LoggedOut: true,
				Next:      pageLogin + "?reason=expired",
			})
			return
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": passwordChangeErrorMessage(err)})
		return
	}

	setAuthCookies(w, r, issued)
	resp, err := s.loadSettingsResponse(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, settingsActionResponse{
		Notice:   "Password updated. Other device sessions were signed out.",
		Settings: &resp,
	})
}

func (s *Server) handleSettingsDeviceRevokeAPI(w http.ResponseWriter, r *http.Request) {
	s.handleSettingsDeviceMutationAPI(w, r, func(req *http.Request, deviceID string, current bool) (settingsDeviceMutationResult, error) {
		if err := authpkg.RevokeDeviceSessions(req.Context(), s.db, deviceID, time.Now().UTC()); err != nil {
			return settingsDeviceMutationResult{}, err
		}
		if current {
			return settingsDeviceMutationResult{
				Notice:    "This browser was signed out.",
				LoggedOut: true,
				Next:      pageLogin + "?reason=logged_out",
			}, nil
		}
		return settingsDeviceMutationResult{Notice: "Device access revoked."}, nil
	})
}

func (s *Server) handleSettingsDeviceBlockAPI(w http.ResponseWriter, r *http.Request) {
	s.handleSettingsDeviceMutationAPI(w, r, func(req *http.Request, deviceID string, current bool) (settingsDeviceMutationResult, error) {
		if err := authpkg.BlockDevice(req.Context(), s.db, deviceID, time.Now().UTC()); err != nil {
			return settingsDeviceMutationResult{}, err
		}
		if current {
			return settingsDeviceMutationResult{
				Notice:    "This browser was blocked.",
				LoggedOut: true,
				Next:      pageLogin + "?reason=blocked",
			}, nil
		}
		return settingsDeviceMutationResult{Notice: "Device blocked."}, nil
	})
}

func (s *Server) handleSettingsDeviceUnblockAPI(w http.ResponseWriter, r *http.Request) {
	s.handleSettingsDeviceMutationAPI(w, r, func(req *http.Request, deviceID string, _ bool) (settingsDeviceMutationResult, error) {
		if err := authpkg.UnblockDevice(req.Context(), s.db, deviceID, time.Now().UTC()); err != nil {
			return settingsDeviceMutationResult{}, err
		}
		return settingsDeviceMutationResult{Notice: "Device unblocked."}, nil
	})
}

func (s *Server) handleSettingsDeviceMutationAPI(
	w http.ResponseWriter,
	r *http.Request,
	mutate func(*http.Request, string, bool) (settingsDeviceMutationResult, error),
) {
	result, err := s.mutateSettingsDevice(r, mutate)
	if err != nil {
		if errors.Is(err, errSettingsSessionRequired) {
			s.writeUnauthorized(w)
			return
		}
		if errors.Is(err, errSettingsDeviceMissing) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": deviceMutationErrorMessage(err)})
		return
	}

	if result.LoggedOut {
		clearAuthCookies(w, r)
		writeJSON(w, http.StatusOK, settingsActionResponse{
			Notice:    result.Notice,
			LoggedOut: true,
			Next:      result.Next,
		})
		return
	}

	resp, err := s.loadSettingsResponse(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, settingsActionResponse{
		Notice:   result.Notice,
		Settings: &resp,
	})
}

func (s *Server) loadSettingsResponse(r *http.Request) (settingsResponse, error) {
	machine, err := s.loadSettingsMachineSnapshot(r.Context())
	if err != nil {
		return settingsResponse{}, err
	}
	passwordConfigured, err := authpkg.PasswordConfigured(r.Context(), s.db)
	if err != nil {
		return settingsResponse{}, fmt.Errorf("failed to load auth state: %w", err)
	}
	project, err := s.projectLayoutData(r)
	if err != nil {
		return settingsResponse{}, fmt.Errorf("failed to load active project: %w", err)
	}
	deviceGroups, err := s.loadSettingsDeviceGroups(r)
	if err != nil {
		return settingsResponse{}, fmt.Errorf("failed to load device access: %w", err)
	}

	return settingsResponse{
		Machine: settingsMachineResponse{
			StorageRoot:           machine.StorageRoot,
			ApprovalMode:          machine.ApprovalMode,
			ApprovalModeLabel:     approvalModeLabel(machine.ApprovalMode),
			HostAccessMode:        machine.HostAccessMode,
			HostAccessModeLabel:   hostAccessModeLabel(machine.HostAccessMode),
			AdminToken:            machine.AdminToken,
			PerRunTokenBudget:     machine.PerRunTokenBudget,
			DailyCostCapUSD:       machine.DailyCostCapUSD,
			RollingCostUSD:        machine.RollingCostUSD,
			RollingCostLabel:      fmt.Sprintf("$%.2f in the last 24h", machine.RollingCostUSD),
			TelegramToken:         machine.TelegramToken,
			WhatsAppPhoneNumberID: machine.WhatsAppPhoneNumberID,
			WhatsAppAccessToken:   machine.WhatsAppAccessToken,
			WhatsAppVerifyToken:   machine.WhatsAppVerifyToken,
			ActiveProjectName:     project.ActiveName,
			ActiveProjectPath:     project.ActiveProjectPath,
			ActiveProjectSummary:  fmt.Sprintf("%s at %s", project.ActiveName, project.ActiveProjectPath),
		},
		Access: settingsAccessResponse{
			PasswordConfigured: passwordConfigured,
			CurrentDevice:      settingsDeviceRowPointerResponse(deviceGroups.CurrentDevice),
			OtherActiveDevices: settingsDeviceRowsResponse(deviceGroups.OtherActiveDevices),
			BlockedDevices:     settingsDeviceRowsResponse(deviceGroups.BlockedDevices),
		},
	}, nil
}

func (s *Server) loadSettingsMachineSnapshot(ctx context.Context) (settingsMachineSnapshot, error) {
	snapshot := settingsMachineSnapshot{
		StorageRoot:       s.storageRoot,
		ApprovalMode:      lookupSetting(s.db, "approval_mode"),
		HostAccessMode:    lookupSetting(s.db, "host_access_mode"),
		PerRunTokenBudget: lookupSetting(s.db, "per_run_token_budget"),
		DailyCostCapUSD:   lookupSetting(s.db, "daily_cost_cap_usd"),
	}
	if snapshot.ApprovalMode == "" {
		snapshot.ApprovalMode = string(authority.ApprovalModePrompt)
	}
	if snapshot.HostAccessMode == "" {
		snapshot.HostAccessMode = string(authority.HostAccessModeStandard)
	}

	if err := s.db.RawDB().QueryRowContext(
		ctx,
		`SELECT COALESCE(SUM(cost_usd), 0) FROM receipts WHERE created_at >= datetime('now', '-24 hours')`,
	).Scan(&snapshot.RollingCostUSD); err != nil {
		return settingsMachineSnapshot{}, fmt.Errorf("failed to load rolling cost: %w", err)
	}

	snapshot.AdminToken = maskSettingsToken(lookupSetting(s.db, "admin_token"))
	snapshot.TelegramToken = maskSettingsToken(lookupSetting(s.db, "telegram_bot_token"))
	snapshot.WhatsAppPhoneNumberID = lookupSetting(s.db, "whatsapp_phone_number_id")
	snapshot.WhatsAppAccessToken = maskSettingsToken(lookupSetting(s.db, "whatsapp_access_token"))
	snapshot.WhatsAppVerifyToken = maskSettingsToken(lookupSetting(s.db, "whatsapp_verify_token"))
	return snapshot, nil
}

func (s *Server) settingsUpdatesFromRequest(req settingsUpdateRequest) (map[string]string, error) {
	approvalMode := ""
	if req.ApprovalMode != nil {
		approvalMode = strings.TrimSpace(*req.ApprovalMode)
	}
	if approvalMode == "" {
		approvalMode = lookupSetting(s.db, "approval_mode")
	}
	if approvalMode == "" {
		approvalMode = string(authority.ApprovalModePrompt)
	}
	switch approvalMode {
	case string(authority.ApprovalModePrompt), string(authority.ApprovalModeAutoApprove):
	default:
		return nil, fmt.Errorf("approval mode is invalid")
	}

	hostAccessMode := ""
	if req.HostAccessMode != nil {
		hostAccessMode = strings.TrimSpace(*req.HostAccessMode)
	}
	if hostAccessMode == "" {
		hostAccessMode = lookupSetting(s.db, "host_access_mode")
	}
	if hostAccessMode == "" {
		hostAccessMode = string(authority.HostAccessModeStandard)
	}
	switch hostAccessMode {
	case string(authority.HostAccessModeStandard), string(authority.HostAccessModeElevated):
	default:
		return nil, fmt.Errorf("host access mode is invalid")
	}

	perRunTokenBudget := lookupSetting(s.db, "per_run_token_budget")
	if req.PerRunTokenBudget != nil {
		perRunTokenBudget = strings.TrimSpace(*req.PerRunTokenBudget)
	}
	dailyCostCapUSD := lookupSetting(s.db, "daily_cost_cap_usd")
	if req.DailyCostCapUSD != nil {
		dailyCostCapUSD = strings.TrimSpace(*req.DailyCostCapUSD)
	}
	telegramBotToken := lookupSetting(s.db, "telegram_bot_token")
	if req.TelegramBotToken != nil {
		telegramBotToken = strings.TrimSpace(*req.TelegramBotToken)
	}
	whatsAppPhoneNumberID := lookupSetting(s.db, "whatsapp_phone_number_id")
	if req.WhatsAppPhoneNumberID != nil {
		whatsAppPhoneNumberID = strings.TrimSpace(*req.WhatsAppPhoneNumberID)
	}
	whatsAppAccessToken := lookupSetting(s.db, "whatsapp_access_token")
	if req.WhatsAppAccessToken != nil {
		whatsAppAccessToken = strings.TrimSpace(*req.WhatsAppAccessToken)
	}
	whatsAppVerifyToken := lookupSetting(s.db, "whatsapp_verify_token")
	if req.WhatsAppVerifyToken != nil {
		whatsAppVerifyToken = strings.TrimSpace(*req.WhatsAppVerifyToken)
	}

	return map[string]string{
		"approval_mode":            approvalMode,
		"host_access_mode":         hostAccessMode,
		"per_run_token_budget":     perRunTokenBudget,
		"daily_cost_cap_usd":       dailyCostCapUSD,
		"telegram_bot_token":       telegramBotToken,
		"whatsapp_phone_number_id": whatsAppPhoneNumberID,
		"whatsapp_access_token":    whatsAppAccessToken,
		"whatsapp_verify_token":    whatsAppVerifyToken,
	}, nil
}

func settingsDeviceRowPointerResponse(row *settingsDeviceRow) *settingsDeviceResponse {
	if row == nil {
		return nil
	}
	resp := settingsDeviceResponse(*row)
	return &resp
}

func settingsDeviceRowsResponse(rows []settingsDeviceRow) []settingsDeviceResponse {
	resp := make([]settingsDeviceResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, settingsDeviceResponse(row))
	}
	return resp
}

func approvalModeLabel(value string) string {
	switch value {
	case string(authority.ApprovalModeAutoApprove):
		return "Auto approve"
	default:
		return "Prompt"
	}
}

func hostAccessModeLabel(value string) string {
	switch value {
	case string(authority.HostAccessModeElevated):
		return "Elevated"
	default:
		return "Standard"
	}
}

func maskSettingsToken(raw string) string {
	if len(raw) > 8 {
		return raw[:8] + strings.Repeat("*", len(raw)-8)
	}
	if raw != "" {
		return strings.Repeat("*", len(raw))
	}
	return ""
}

func settingsUpdateErrorMessage(err error) string {
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "per_run_token_budget"):
		return "Per-run token budget must be a whole number."
	case strings.Contains(message, "daily_cost_cap_usd"):
		return "Daily cost cap must be a number."
	default:
		return message
	}
}
