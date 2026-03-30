package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
)

func TestGETLoginServesSPAIndex(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	wantBody, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read spa index: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, pageLogin, nil)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != string(wantBody) {
		t.Fatalf("expected login route to serve spa index")
	}
}

func TestAuthenticatedGETChatServesSPAIndex(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Now().UTC()); err != nil {
		t.Fatalf("set password: %v", err)
	}
	sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

	wantBody, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read spa index: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != string(wantBody) {
		t.Fatalf("expected chat route to serve spa index")
	}
}

func TestAuthenticatedGETNestedSPARoutesServeIndex(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Now().UTC()); err != nil {
		t.Fatalf("set password: %v", err)
	}
	sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

	wantBody, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read spa index: %v", err)
	}

	for _, path := range []string{"/chat", "/sessions"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(sessionCookie)
		req.AddCookie(deviceCookie)
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d body=%s", path, rr.Code, rr.Body.String())
		}
		if rr.Body.String() != string(wantBody) {
			t.Fatalf("expected %s to serve spa index", path)
		}
	}
}

func TestAuthSessionAPIReportsSetupAndDeviceState(t *testing.T) {
	t.Parallel()

	t.Run("setup required when password missing", func(t *testing.T) {
		t.Parallel()

		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Authenticated      bool   `json:"authenticated"`
			PasswordConfigured bool   `json:"password_configured"`
			SetupRequired      bool   `json:"setup_required"`
			DeviceID           string `json:"device_id"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Authenticated {
			t.Fatal("expected unauthenticated response")
		}
		if resp.PasswordConfigured {
			t.Fatal("expected password_configured false")
		}
		if !resp.SetupRequired {
			t.Fatal("expected setup_required true")
		}
		if resp.DeviceID != "" {
			t.Fatalf("expected empty device id, got %q", resp.DeviceID)
		}
	})

	t.Run("authenticated session includes device id", func(t *testing.T) {
		t.Parallel()

		h := newServerHarness(t)
		if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Now().UTC()); err != nil {
			t.Fatalf("set password: %v", err)
		}
		sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
		req.AddCookie(sessionCookie)
		req.AddCookie(deviceCookie)
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Authenticated      bool   `json:"authenticated"`
			PasswordConfigured bool   `json:"password_configured"`
			SetupRequired      bool   `json:"setup_required"`
			DeviceID           string `json:"device_id"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !resp.Authenticated {
			t.Fatal("expected authenticated response")
		}
		if !resp.PasswordConfigured {
			t.Fatal("expected password_configured true")
		}
		if resp.SetupRequired {
			t.Fatal("expected setup_required false")
		}
		if resp.DeviceID == "" {
			t.Fatal("expected non-empty device id")
		}
	})
}

func TestAuthLoginAPIAuthenticatesAndSetsCookies(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Now().UTC()); err != nil {
		t.Fatalf("set password: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"password":"secret-pass","next":"/chat"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GistClaw Test Browser")
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Authenticated bool   `json:"authenticated"`
		Next          string `json:"next"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Authenticated {
		t.Fatal("expected authenticated response")
	}
	if resp.Next != "/chat" {
		t.Fatalf("next = %q, want %q", resp.Next, "/chat")
	}
	if findCookie(rr.Result().Cookies(), sessionCookieName) == nil {
		t.Fatal("expected session cookie to be set")
	}
	if findCookie(rr.Result().Cookies(), deviceCookieName) == nil {
		t.Fatal("expected device cookie to be set")
	}
}

func TestAuthLoginAPIDefaultsToOnboardingWhenPending(t *testing.T) {
	t.Parallel()

	h := newServerHarnessOnboardingPending(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Now().UTC()); err != nil {
		t.Fatalf("set password: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"password":"secret-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GistClaw Test Browser")
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Next string `json:"next"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Next != pageOnboarding {
		t.Fatalf("next = %q, want %q", resp.Next, pageOnboarding)
	}
}

func TestAuthLoginAPIRejectsInvalidPassword(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Now().UTC()); err != nil {
		t.Fatalf("set password: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"password":"wrong-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Authenticated bool   `json:"authenticated"`
		Next          string `json:"next"`
		Message       string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Authenticated {
		t.Fatal("expected unauthenticated response")
	}
	if resp.Message == "" {
		t.Fatal("expected error message")
	}
	if resp.Next != "" {
		t.Fatalf("expected empty next on login rejection, got %q", resp.Next)
	}
	if findCookie(rr.Result().Cookies(), sessionCookieName) != nil {
		t.Fatal("expected no session cookie on login rejection")
	}
	if findCookie(rr.Result().Cookies(), deviceCookieName) != nil {
		t.Fatal("expected no device cookie on login rejection")
	}
}

func TestAuthLogoutAPIClearsCookiesAndInvalidatesSession(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Now().UTC()); err != nil {
		t.Fatalf("set password: %v", err)
	}
	sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		LoggedOut bool `json:"logged_out"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.LoggedOut {
		t.Fatal("expected logged_out true")
	}

	for _, name := range []string{sessionCookieName, deviceCookieName} {
		cookie := findCookie(rr.Result().Cookies(), name)
		if cookie == nil || cookie.MaxAge != -1 {
			t.Fatalf("expected %s cookie to be cleared, got %#v", name, cookie)
		}
	}

	sessionRR := httptest.NewRecorder()
	sessionReq := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	sessionReq.AddCookie(sessionCookie)
	sessionReq.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(sessionRR, sessionReq)

	if sessionRR.Code != http.StatusOK {
		t.Fatalf("expected 200 checking session, got %d body=%s", sessionRR.Code, sessionRR.Body.String())
	}

	var sessionResp struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(sessionRR.Body.Bytes(), &sessionResp); err != nil {
		t.Fatalf("decode post-logout session response: %v", err)
	}
	if sessionResp.Authenticated {
		t.Fatal("expected session to be invalidated")
	}
}
