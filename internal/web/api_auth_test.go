package web

import (
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

func TestAuthenticatedGETWorkServesSPAIndex(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != string(wantBody) {
		t.Fatalf("expected work route to serve spa index")
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
