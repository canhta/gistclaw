// internal/providers/codex/token_refresh_test.go
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

func newTestStoreCodex(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestEnsureToken_RefreshesWithinBuffer verifies that ensureToken treats a token
// expiring within tokenRefreshBuffer (5 min) as stale and triggers a refresh.
func TestEnsureToken_RefreshesWithinBuffer(t *testing.T) {
	refreshCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalled = true
		w.Header().Set("Content-Type", "application/json")
		expiry := time.Now().Add(1 * time.Hour).Unix()
		fmt.Fprintf(w, `{"access_token":"new-token","token_type":"Bearer","expires_in":3600,"expiry":%d}`, expiry)
	}))
	defer srv.Close()

	s := newTestStoreCodex(t)
	p := NewWithURLs(s, srv.URL, "http://unused-chat")

	tok := StoredToken{
		AccessToken:  "old-token",
		RefreshToken: "some-refresh-token",
		ExpiresAt:    time.Now().Add(3 * time.Minute),
	}
	raw, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := s.SetProviderCredentials("codex", string(raw)); err != nil {
		t.Fatalf("SetProviderCredentials: %v", err)
	}

	result, err := p.ensureToken(context.Background())
	if err != nil {
		t.Fatalf("ensureToken: %v", err)
	}
	if !refreshCalled {
		t.Error("token refresh endpoint was NOT called; token within 5-min buffer should trigger refresh")
	}
	if result.AccessToken == "old-token" {
		t.Error("ensureToken returned stale token; expected refreshed token")
	}
}

// TestEnsureToken_ValidTokenNotRefreshed verifies that a token with >5 min remaining
// is returned from cache without calling the refresh endpoint.
func TestEnsureToken_ValidTokenNotRefreshed(t *testing.T) {
	refreshCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := newTestStoreCodex(t)
	p := NewWithURLs(s, srv.URL, "http://unused-chat")

	tok := StoredToken{
		AccessToken:  "valid-token",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	raw, _ := json.Marshal(tok)
	_ = s.SetProviderCredentials("codex", string(raw))

	result, err := p.ensureToken(context.Background())
	if err != nil {
		t.Fatalf("ensureToken: %v", err)
	}
	if refreshCalled {
		t.Error("refresh endpoint was called unexpectedly for a valid token")
	}
	if result.AccessToken != "valid-token" {
		t.Errorf("expected valid-token; got %q", result.AccessToken)
	}
}
