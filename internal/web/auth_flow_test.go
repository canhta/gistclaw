package web

import (
	"context"
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

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Local setup required", "gistclaw auth set-password"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected login page to contain %q, got:\n%s", want, body)
		}
	}
	if strings.Contains(body, `href="/operate/runs"`) {
		t.Fatalf("expected stripped pre-auth shell, got:\n%s", body)
	}
}

func TestAuthProtectedPagesRedirectToLogin(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?") || !strings.Contains(loc, "next=%2Foperate%2Fruns") {
		t.Fatalf("expected redirect to login with next path, got %q", loc)
	}
}

func TestAuthLoginCreatesSessionCookiesAndUnlocksOperatorPages(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 10, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	form := url.Values{
		"password": {"secret-pass"},
		"next":     {"/operate/runs"},
	}
	loginResp := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodPost, "http://localhost/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	h.rawServer.ServeHTTP(loginResp, loginReq)

	if loginResp.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", loginResp.Code, loginResp.Body.String())
	}
	if loginResp.Header().Get("Location") != "/operate/runs" {
		t.Fatalf("expected redirect to /operate/runs, got %q", loginResp.Header().Get("Location"))
	}
	sessionCookie := findCookie(loginResp.Result().Cookies(), "gistclaw_session")
	deviceCookie := findCookie(loginResp.Result().Cookies(), "gistclaw_device")
	if sessionCookie == nil {
		t.Fatal("expected gistclaw_session cookie")
	}
	if deviceCookie == nil {
		t.Fatal("expected gistclaw_device cookie")
	}

	pageResp := httptest.NewRecorder()
	pageReq := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)
	pageReq.AddCookie(sessionCookie)
	pageReq.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(pageResp, pageReq)

	if pageResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated page load 200, got %d", pageResp.Code)
	}
	if !strings.Contains(pageResp.Body.String(), "Runs") {
		t.Fatalf("expected runs page, got:\n%s", pageResp.Body.String())
	}
}

func TestAuthReadAPIsRequireSessionOrBearer(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 20, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	anonResp := httptest.NewRecorder()
	anonReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	h.rawServer.ServeHTTP(anonResp, anonReq)
	if anonResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected anonymous API request to be 401, got %d", anonResp.Code)
	}

	bearerResp := httptest.NewRecorder()
	bearerReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	bearerReq.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.rawServer.ServeHTTP(bearerResp, bearerReq)
	if bearerResp.Code != http.StatusOK {
		t.Fatalf("expected bearer API request to succeed, got %d body=%s", bearerResp.Code, bearerResp.Body.String())
	}
}

func TestAuthSettingsPageShowsAccessAndDevicesBoard(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 30, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/configure/settings", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Access &amp; Devices", "Current Device", "Other Active Devices", "Machine Settings"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected settings page to contain %q, got:\n%s", want, body)
		}
	}
}

func TestAuthLoginReasonMessages(t *testing.T) {
	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Date(2026, time.March, 27, 7, 40, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	cases := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "expired", reason: "expired", want: "Your session expired. Sign in again to continue."},
		{name: "logged out", reason: "logged_out", want: "You signed out of this browser."},
		{name: "blocked", reason: "blocked", want: "This device has been blocked."},
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
			if !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("expected login page to contain %q, got:\n%s", tc.want, rr.Body.String())
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

	form := url.Values{
		"current_password": {"secret-pass"},
		"new_password":     {"new-secret-pass"},
		"confirm_password": {"new-secret-pass"},
	}
	changeResp := httptest.NewRecorder()
	changeReq := httptest.NewRequest(http.MethodPost, "http://localhost/configure/settings/password", strings.NewReader(form.Encode()))
	changeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	changeReq.Header.Set("Origin", "http://localhost")
	changeReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	changeReq.AddCookie(sessionCookie)
	changeReq.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(changeResp, changeReq)

	if changeResp.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", changeResp.Code, changeResp.Body.String())
	}
	location := changeResp.Header().Get("Location")
	if !strings.Contains(location, "access_notice=") {
		t.Fatalf("expected settings notice redirect, got %q", location)
	}
	newSessionCookie := findCookie(changeResp.Result().Cookies(), sessionCookieName)
	newDeviceCookie := findCookie(changeResp.Result().Cookies(), deviceCookieName)
	if newSessionCookie == nil || newDeviceCookie == nil {
		t.Fatalf("expected refreshed auth cookies, got session=%v device=%v", newSessionCookie, newDeviceCookie)
	}

	noticeResp := httptest.NewRecorder()
	noticeReq := httptest.NewRequest(http.MethodGet, location, nil)
	noticeReq.AddCookie(newSessionCookie)
	noticeReq.AddCookie(newDeviceCookie)
	h.rawServer.ServeHTTP(noticeResp, noticeReq)
	if noticeResp.Code != http.StatusOK {
		t.Fatalf("expected redirected settings page 200, got %d", noticeResp.Code)
	}
	if !strings.Contains(noticeResp.Body.String(), "Password updated. Other device sessions were signed out.") {
		t.Fatalf("expected password update notice, got:\n%s", noticeResp.Body.String())
	}

	oldLoginResp := httptest.NewRecorder()
	oldLoginReq := httptest.NewRequest(http.MethodPost, "http://localhost/login", strings.NewReader(url.Values{"password": {"secret-pass"}}.Encode()))
	oldLoginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.rawServer.ServeHTTP(oldLoginResp, oldLoginReq)
	if oldLoginResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password to fail, got %d", oldLoginResp.Code)
	}

	newLoginResp := httptest.NewRecorder()
	newLoginReq := httptest.NewRequest(http.MethodPost, "http://localhost/login", strings.NewReader(url.Values{"password": {"new-secret-pass"}}.Encode()))
	newLoginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.rawServer.ServeHTTP(newLoginResp, newLoginReq)
	if newLoginResp.Code != http.StatusSeeOther {
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

	form := url.Values{"password": {password}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected login redirect, got %d body=%s", rr.Code, rr.Body.String())
	}

	sessionCookie := findCookie(rr.Result().Cookies(), "gistclaw_session")
	deviceCookie := findCookie(rr.Result().Cookies(), "gistclaw_device")
	if sessionCookie == nil || deviceCookie == nil {
		t.Fatalf("expected auth cookies, got session=%v device=%v", sessionCookie, deviceCookie)
	}
	return sessionCookie, deviceCookie
}
