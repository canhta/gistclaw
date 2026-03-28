package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
	"github.com/canhta/gistclaw/internal/authority"
)

func TestSettingsAPIListsMachineFactsAndAccessBoard(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 28, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := h.rt.UpdateSettings(context.Background(), map[string]string{
		"approval_mode":        string(authority.ApprovalModeAutoApprove),
		"host_access_mode":     string(authority.HostAccessModeElevated),
		"per_run_token_budget": "100000",
		"daily_cost_cap_usd":   "5.5",
		"telegram_bot_token":   "12345678-secret-token",
	}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if _, err := h.db.RawDB().Exec(
		`INSERT INTO receipts (id, run_id, cost_usd, created_at) VALUES (?, ?, ?, ?)`,
		"receipt-settings-1",
		"run-settings-1",
		2.75,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	currentSession, currentDevice := loginForTest(t, h, "secret-pass")
	activeSession, _ := loginForTestWithUserAgent(t, h, "secret-pass", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/123.0.0.0")
	blockedSession, _ := loginForTestWithUserAgent(t, h, "secret-pass", "Mozilla/5.0 (X11; Linux x86_64) Firefox/124.0")

	blockedDeviceID := deviceIDFromSessionCookie(t, h, blockedSession)
	if err := authpkg.BlockDevice(context.Background(), h.db, blockedDeviceID, time.Now().UTC()); err != nil {
		t.Fatalf("BlockDevice: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/api/settings", nil)
	req.AddCookie(currentSession)
	req.AddCookie(currentDevice)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Machine struct {
			StorageRoot       string  `json:"storage_root"`
			ApprovalMode      string  `json:"approval_mode"`
			HostAccessMode    string  `json:"host_access_mode"`
			AdminToken        string  `json:"admin_token"`
			PerRunTokenBudget string  `json:"per_run_token_budget"`
			DailyCostCapUSD   string  `json:"daily_cost_cap_usd"`
			RollingCostUSD    float64 `json:"rolling_cost_usd"`
			TelegramToken     string  `json:"telegram_token"`
		} `json:"machine"`
		Access struct {
			PasswordConfigured bool `json:"password_configured"`
			CurrentDevice      *struct {
				ID      string `json:"id"`
				Current bool   `json:"current"`
			} `json:"current_device"`
			OtherActiveDevices []struct {
				ID      string `json:"id"`
				Blocked bool   `json:"blocked"`
			} `json:"other_active_devices"`
			BlockedDevices []struct {
				ID      string `json:"id"`
				Blocked bool   `json:"blocked"`
			} `json:"blocked_devices"`
		} `json:"access"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	currentDeviceID := deviceIDFromSessionCookie(t, h, currentSession)
	activeDeviceID := deviceIDFromSessionCookie(t, h, activeSession)

	if !resp.Access.PasswordConfigured {
		t.Fatal("expected password_configured true")
	}
	if resp.Access.CurrentDevice == nil || resp.Access.CurrentDevice.ID != currentDeviceID || !resp.Access.CurrentDevice.Current {
		t.Fatalf("unexpected current device %+v", resp.Access.CurrentDevice)
	}
	if len(resp.Access.OtherActiveDevices) != 1 || resp.Access.OtherActiveDevices[0].ID != activeDeviceID || resp.Access.OtherActiveDevices[0].Blocked {
		t.Fatalf("unexpected other active devices %+v", resp.Access.OtherActiveDevices)
	}
	if len(resp.Access.BlockedDevices) != 1 || resp.Access.BlockedDevices[0].ID != blockedDeviceID || !resp.Access.BlockedDevices[0].Blocked {
		t.Fatalf("unexpected blocked devices %+v", resp.Access.BlockedDevices)
	}
	if resp.Machine.StorageRoot != h.storageRoot {
		t.Fatalf("storage_root = %q, want %q", resp.Machine.StorageRoot, h.storageRoot)
	}
	if resp.Machine.ApprovalMode != string(authority.ApprovalModeAutoApprove) {
		t.Fatalf("approval_mode = %q", resp.Machine.ApprovalMode)
	}
	if resp.Machine.HostAccessMode != string(authority.HostAccessModeElevated) {
		t.Fatalf("host_access_mode = %q", resp.Machine.HostAccessMode)
	}
	if resp.Machine.PerRunTokenBudget != "100000" || resp.Machine.DailyCostCapUSD != "5.5" {
		t.Fatalf("unexpected budget settings %+v", resp.Machine)
	}
	if resp.Machine.RollingCostUSD != 2.75 {
		t.Fatalf("rolling_cost_usd = %v", resp.Machine.RollingCostUSD)
	}
	if !strings.HasPrefix(resp.Machine.TelegramToken, "12345678") || strings.Contains(resp.Machine.TelegramToken, "secret-token") {
		t.Fatalf("telegram_token = %q", resp.Machine.TelegramToken)
	}
	if resp.Machine.AdminToken != "test-adm********" {
		t.Fatalf("admin_token = %q", resp.Machine.AdminToken)
	}
}

func TestSettingsAPIMutationsUpdateMachineFactsPasswordAndDeviceBoard(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 28, 8, 10, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	currentSession, currentDevice := loginForTest(t, h, "secret-pass")
	otherSession, _ := loginForTestWithUserAgent(t, h, "secret-pass", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/123.0.0.0")
	otherDeviceID := deviceIDFromSessionCookie(t, h, otherSession)

	t.Run("updates machine settings", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/api/settings", strings.NewReader(`{
			"approval_mode":"auto_approve",
			"host_access_mode":"elevated",
			"per_run_token_budget":"50000",
			"daily_cost_cap_usd":"3.25",
			"telegram_bot_token":"87654321-settings-token"
		}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(currentSession)
		req.AddCookie(currentDevice)
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Notice   string `json:"notice"`
			Settings struct {
				Machine struct {
					ApprovalMode      string `json:"approval_mode"`
					HostAccessMode    string `json:"host_access_mode"`
					PerRunTokenBudget string `json:"per_run_token_budget"`
					DailyCostCapUSD   string `json:"daily_cost_cap_usd"`
					TelegramToken     string `json:"telegram_token"`
				} `json:"machine"`
			} `json:"settings"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Notice != "Machine settings updated." {
			t.Fatalf("notice = %q", resp.Notice)
		}
		if resp.Settings.Machine.ApprovalMode != "auto_approve" || resp.Settings.Machine.HostAccessMode != "elevated" {
			t.Fatalf("unexpected machine modes %+v", resp.Settings.Machine)
		}
		if resp.Settings.Machine.PerRunTokenBudget != "50000" || resp.Settings.Machine.DailyCostCapUSD != "3.25" {
			t.Fatalf("unexpected budget settings %+v", resp.Settings.Machine)
		}
		if !strings.HasPrefix(resp.Settings.Machine.TelegramToken, "87654321") || strings.Contains(resp.Settings.Machine.TelegramToken, "settings-token") {
			t.Fatalf("telegram_token = %q", resp.Settings.Machine.TelegramToken)
		}
	})

	t.Run("blocks and unblocks another browser", func(t *testing.T) {
		blockRR := httptest.NewRecorder()
		blockReq := httptest.NewRequest(http.MethodPost, "http://localhost/api/settings/devices/"+otherDeviceID+"/block", nil)
		blockReq.Header.Set("Origin", "http://localhost")
		blockReq.AddCookie(currentSession)
		blockReq.AddCookie(currentDevice)
		h.rawServer.ServeHTTP(blockRR, blockReq)

		if blockRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", blockRR.Code, blockRR.Body.String())
		}

		var blockResp struct {
			Notice   string `json:"notice"`
			Settings struct {
				Access struct {
					BlockedDevices []struct {
						ID string `json:"id"`
					} `json:"blocked_devices"`
				} `json:"access"`
			} `json:"settings"`
		}
		if err := json.Unmarshal(blockRR.Body.Bytes(), &blockResp); err != nil {
			t.Fatalf("decode block response: %v", err)
		}
		if blockResp.Notice != "Device blocked." {
			t.Fatalf("unexpected block notice %q", blockResp.Notice)
		}
		if len(blockResp.Settings.Access.BlockedDevices) != 1 || blockResp.Settings.Access.BlockedDevices[0].ID != otherDeviceID {
			t.Fatalf("unexpected blocked devices %+v", blockResp.Settings.Access.BlockedDevices)
		}

		unblockRR := httptest.NewRecorder()
		unblockReq := httptest.NewRequest(http.MethodPost, "http://localhost/api/settings/devices/"+otherDeviceID+"/unblock", nil)
		unblockReq.Header.Set("Origin", "http://localhost")
		unblockReq.AddCookie(currentSession)
		unblockReq.AddCookie(currentDevice)
		h.rawServer.ServeHTTP(unblockRR, unblockReq)

		if unblockRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", unblockRR.Code, unblockRR.Body.String())
		}

		var unblockResp struct {
			Notice   string `json:"notice"`
			Settings struct {
				Access struct {
					BlockedDevices []struct {
						ID string `json:"id"`
					} `json:"blocked_devices"`
				} `json:"access"`
			} `json:"settings"`
		}
		if err := json.Unmarshal(unblockRR.Body.Bytes(), &unblockResp); err != nil {
			t.Fatalf("decode unblock response: %v", err)
		}
		if unblockResp.Notice != "Device unblocked." {
			t.Fatalf("unexpected unblock notice %q", unblockResp.Notice)
		}
		if len(unblockResp.Settings.Access.BlockedDevices) != 0 {
			t.Fatalf("unexpected blocked devices %+v", unblockResp.Settings.Access.BlockedDevices)
		}
	})

	t.Run("changes the password and signs out other browsers", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/api/settings/password", strings.NewReader(`{
			"current_password":"secret-pass",
			"new_password":"new-secret-pass",
			"confirm_password":"new-secret-pass"
		}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
		req.AddCookie(currentSession)
		req.AddCookie(currentDevice)
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Notice string `json:"notice"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode password response: %v", err)
		}
		if resp.Notice != "Password updated. Other device sessions were signed out." {
			t.Fatalf("notice = %q", resp.Notice)
		}
		if findCookie(rr.Result().Cookies(), sessionCookieName) == nil || findCookie(rr.Result().Cookies(), deviceCookieName) == nil {
			t.Fatalf("expected refreshed auth cookies after password change")
		}

		oldOtherResp := httptest.NewRecorder()
		oldOtherReq := httptest.NewRequest(http.MethodGet, "http://localhost/api/settings", nil)
		oldOtherReq.AddCookie(otherSession)
		h.rawServer.ServeHTTP(oldOtherResp, oldOtherReq)
		if oldOtherResp.Code != http.StatusUnauthorized {
			t.Fatalf("expected old other session to be unauthorized, got %d body=%s", oldOtherResp.Code, oldOtherResp.Body.String())
		}
	})
}

func TestSettingsAPICurrentBrowserMutationLogsOut(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 28, 8, 20, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	currentSession, currentDevice := loginForTest(t, h, "secret-pass")
	currentDeviceID := deviceIDFromSessionCookie(t, h, currentSession)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/settings/devices/"+currentDeviceID+"/block", nil)
	req.Header.Set("Origin", "http://localhost")
	req.AddCookie(currentSession)
	req.AddCookie(currentDevice)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Notice    string `json:"notice"`
		LoggedOut bool   `json:"logged_out"`
		Next      string `json:"next"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.LoggedOut {
		t.Fatal("expected logged_out true")
	}
	if resp.Next != "/login?reason=blocked" {
		t.Fatalf("next = %q", resp.Next)
	}
	if resp.Notice != "This browser was blocked." {
		t.Fatalf("notice = %q", resp.Notice)
	}
	for _, name := range []string{sessionCookieName, deviceCookieName} {
		cookie := findCookie(rr.Result().Cookies(), name)
		if cookie == nil || cookie.MaxAge != -1 {
			t.Fatalf("expected %s to be cleared, got %#v", name, cookie)
		}
	}
}

func loginForTestWithUserAgent(t *testing.T, h *serverHarness, password, userAgent string) (*http.Cookie, *http.Cookie) {
	t.Helper()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/login", strings.NewReader("password="+password))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected login redirect, got %d body=%s", rr.Code, rr.Body.String())
	}

	sessionCookie := findCookie(rr.Result().Cookies(), sessionCookieName)
	deviceCookie := findCookie(rr.Result().Cookies(), deviceCookieName)
	if sessionCookie == nil || deviceCookie == nil {
		t.Fatalf("expected auth cookies, got session=%v device=%v", sessionCookie, deviceCookie)
	}
	return sessionCookie, deviceCookie
}

func deviceIDFromSessionCookie(t *testing.T, h *serverHarness, sessionCookie *http.Cookie) string {
	t.Helper()

	state, err := authpkg.AuthenticateSession(context.Background(), h.db, sessionCookie.Value, time.Now().UTC())
	if err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	return state.DeviceID
}
