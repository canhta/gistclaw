package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
)

func TestAuthLoginPageShowsSetupRequiredWhenPasswordMissing(t *testing.T) {
	h := newServerHarness(t)
	wantBody, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read spa index: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != string(wantBody) {
		t.Fatalf("expected login route to serve spa index")
	}

	stateResp := httptest.NewRecorder()
	stateReq := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	h.rawServer.ServeHTTP(stateResp, stateReq)
	if stateResp.Code != http.StatusOK {
		t.Fatalf("expected auth session 200, got %d body=%s", stateResp.Code, stateResp.Body.String())
	}

	var state struct {
		Authenticated      bool `json:"authenticated"`
		PasswordConfigured bool `json:"password_configured"`
		SetupRequired      bool `json:"setup_required"`
	}
	if err := json.Unmarshal(stateResp.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode auth session: %v", err)
	}
	if state.Authenticated {
		t.Fatal("expected unauthenticated session")
	}
	if state.PasswordConfigured {
		t.Fatal("expected password_configured false")
	}
	if !state.SetupRequired {
		t.Fatal("expected setup_required true")
	}
}

func TestAuthLoginPageAccessibleWhenOnboardingPending(t *testing.T) {
	h := newServerHarnessOnboardingPending(t)
	wantBody, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read spa index: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != string(wantBody) {
		t.Fatalf("expected login route to serve spa index while onboarding is pending")
	}
}

func TestAuthProtectedPagesRedirectToLogin(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?") || !strings.Contains(loc, "next=%2Fwork") {
		t.Fatalf("expected redirect to login with next path, got %q", loc)
	}
}

func TestAuthLoginCreatesSessionCookiesAndUnlocksOperatorPages(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 10, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	wantBody, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read spa index: %v", err)
	}

	sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

	pageResp := httptest.NewRecorder()
	pageReq := httptest.NewRequest(http.MethodGet, "/work", nil)
	pageReq.AddCookie(sessionCookie)
	pageReq.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(pageResp, pageReq)

	if pageResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated page load 200, got %d", pageResp.Code)
	}
	if pageResp.Body.String() != string(wantBody) {
		t.Fatalf("expected work route to serve spa index")
	}
}

func TestAuthReadAPIsRequireSessionOrBearer(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 20, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	anonResp := httptest.NewRecorder()
	anonReq := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
	h.rawServer.ServeHTTP(anonResp, anonReq)
	if anonResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected anonymous API request to be 401, got %d", anonResp.Code)
	}

	bearerResp := httptest.NewRecorder()
	bearerReq := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
	bearerReq.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.rawServer.ServeHTTP(bearerResp, bearerReq)
	if bearerResp.Code != http.StatusOK {
		t.Fatalf("expected bearer API request to succeed, got %d body=%s", bearerResp.Code, bearerResp.Body.String())
	}
}

func TestAuthSettingsPageServesSPAWhenAuthenticated(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 30, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	wantBody, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read spa index: %v", err)
	}

	sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != string(wantBody) {
		t.Fatalf("expected settings route to serve spa index")
	}
}

func TestAuthLoginRouteServesSPAForReasonQuery(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 40, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	wantBody, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read spa index: %v", err)
	}

	cases := []struct {
		name   string
		reason string
	}{
		{name: "expired", reason: "expired"},
		{name: "logged out", reason: "logged_out"},
		{name: "blocked", reason: "blocked"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/login?reason="+url.QueryEscape(tc.reason), nil)
			h.rawServer.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rr.Code)
			}
			if rr.Body.String() != string(wantBody) {
				t.Fatalf("expected login route to serve spa index for reason=%q", tc.reason)
			}
		})
	}
}

func TestAuthPasswordChangeRotatesBrowserPassword(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 50, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

	changeResp := httptest.NewRecorder()
	changeReq := httptest.NewRequest(
		http.MethodPost,
		"http://localhost/api/settings/password",
		strings.NewReader(`{"current_password":"secret-pass","new_password":"new-secret-pass","confirm_password":"new-secret-pass"}`),
	)
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.Header.Set("Origin", "http://localhost")
	changeReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	changeReq.AddCookie(sessionCookie)
	changeReq.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(changeResp, changeReq)

	if changeResp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", changeResp.Code, changeResp.Body.String())
	}
	newSessionCookie := findCookie(changeResp.Result().Cookies(), sessionCookieName)
	newDeviceCookie := findCookie(changeResp.Result().Cookies(), deviceCookieName)
	if newSessionCookie == nil || newDeviceCookie == nil {
		t.Fatalf("expected refreshed auth cookies, got session=%v device=%v", newSessionCookie, newDeviceCookie)
	}

	oldLoginResp := httptest.NewRecorder()
	oldLoginReq := httptest.NewRequest(http.MethodPost, "http://localhost/api/auth/login", strings.NewReader(`{"password":"secret-pass"}`))
	oldLoginReq.Header.Set("Content-Type", "application/json")
	h.rawServer.ServeHTTP(oldLoginResp, oldLoginReq)
	if oldLoginResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password to fail with 401, got %d body=%s", oldLoginResp.Code, oldLoginResp.Body.String())
	}

	newLoginResp := httptest.NewRecorder()
	newLoginReq := httptest.NewRequest(http.MethodPost, "http://localhost/api/auth/login", strings.NewReader(`{"password":"new-secret-pass"}`))
	newLoginReq.Header.Set("Content-Type", "application/json")
	h.rawServer.ServeHTTP(newLoginResp, newLoginReq)
	if newLoginResp.Code != http.StatusOK {
		t.Fatalf("expected new password login to succeed, got %d body=%s", newLoginResp.Code, newLoginResp.Body.String())
	}
}

func TestAuthLogoutClearsCookiesAndRedirectsToLogin(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

	logoutResp := httptest.NewRecorder()
	logoutReq := httptest.NewRequest(http.MethodPost, "http://localhost/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutReq.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(logoutResp, logoutReq)

	if logoutResp.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", logoutResp.Code)
	}
	if logoutResp.Header().Get("Location") != "/login?reason=logged_out" {
		t.Fatalf("expected logout redirect, got %q", logoutResp.Header().Get("Location"))
	}
	for _, name := range []string{sessionCookieName, deviceCookieName} {
		cookie := findCookie(logoutResp.Result().Cookies(), name)
		if cookie == nil || cookie.MaxAge != -1 {
			t.Fatalf("expected %s to be cleared, got %#v", name, cookie)
		}
	}
}

func loginForTest(t *testing.T, h *serverHarness, password string) (*http.Cookie, *http.Cookie) {
	t.Helper()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/auth/login", strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected login success, got %d body=%s", rr.Code, rr.Body.String())
	}

	sessionCookie := findCookie(rr.Result().Cookies(), "gistclaw_session")
	deviceCookie := findCookie(rr.Result().Cookies(), "gistclaw_device")
	if sessionCookie == nil || deviceCookie == nil {
		t.Fatalf("expected auth cookies, got session=%v device=%v", sessionCookie, deviceCookie)
	}
	return sessionCookie, deviceCookie
}
