// internal/providers/codex/codex_test.go
package codex_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/providers"
	codexprovider "github.com/canhta/gistclaw/internal/providers/codex"
	"github.com/canhta/gistclaw/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() }) //nolint:errcheck
	return s
}

// mockTokenServer serves a minimal OAuth2 token response.
func mockTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"access_token":  "test-access-token",
			"token_type":    "Bearer",
			"refresh_token": "test-refresh-token",
			"expires_in":    3600,
		})
	}))
}

// mockChatServer serves a minimal chatgpt.com /backend-api/conversation response.
func mockChatServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Authorization header is present.
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header on chat request")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"message": map[string]any{
				"content": map[string]any{
					"parts": []string{content},
				},
			},
		})
	}))
}

func TestCodexName(t *testing.T) {
	s := newTestStore(t)
	p := codexprovider.NewWithURLs(s, "http://unused", "http://unused")
	if p.Name() != "codex" {
		t.Errorf("Name() = %q, want %q", p.Name(), "codex")
	}
}

func TestCodexChatWithStoredToken(t *testing.T) {
	s := newTestStore(t)

	// Pre-seed the store with a valid (non-expired) token so no PKCE flow is triggered.
	token := codexprovider.StoredToken{
		AccessToken:  "pre-seeded-token",
		RefreshToken: "pre-seeded-refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	tokenJSON, _ := json.Marshal(token)
	if err := s.SetProviderCredentials("codex", string(tokenJSON)); err != nil {
		t.Fatalf("SetProviderCredentials: %v", err)
	}

	chatSrv := mockChatServer(t, "Hello from Codex!")
	defer chatSrv.Close()

	p := codexprovider.NewWithURLs(s, "http://unused-token", chatSrv.URL+"/backend-api/conversation")

	resp, err := p.Chat(context.Background(), []providers.Message{
		{Role: "user", Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.Content != "Hello from Codex!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello from Codex!")
	}
	// Codex is unofficial; billing is opaque.
	if resp.Usage.TotalCostUSD != 0 {
		t.Errorf("TotalCostUSD = %v, want 0", resp.Usage.TotalCostUSD)
	}
}

func TestCodexTokenPersistence(t *testing.T) {
	s := newTestStore(t)

	// Seed an expired token to trigger a refresh.
	token := codexprovider.StoredToken{
		AccessToken:  "old-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired
	}
	tokenJSON, _ := json.Marshal(token)
	if err := s.SetProviderCredentials("codex", string(tokenJSON)); err != nil {
		t.Fatalf("SetProviderCredentials: %v", err)
	}

	tokenSrv := mockTokenServer(t)
	defer tokenSrv.Close()
	chatSrv := mockChatServer(t, "refreshed response")
	defer chatSrv.Close()

	p := codexprovider.NewWithURLs(s, tokenSrv.URL+"/oauth/token", chatSrv.URL+"/backend-api/conversation")

	resp, err := p.Chat(context.Background(), []providers.Message{
		{Role: "user", Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat after token refresh: %v", err)
	}
	if resp.Content != "refreshed response" {
		t.Errorf("Content = %q", resp.Content)
	}

	// Verify the new token was persisted.
	stored, err := s.GetProviderCredentials("codex")
	if err != nil {
		t.Fatalf("GetProviderCredentials after refresh: %v", err)
	}
	var newToken codexprovider.StoredToken
	if err := json.Unmarshal([]byte(stored), &newToken); err != nil {
		t.Fatalf("unmarshal stored token: %v", err)
	}
	if newToken.AccessToken != "test-access-token" {
		t.Errorf("persisted AccessToken = %q, want %q", newToken.AccessToken, "test-access-token")
	}
}

func TestCodexInterfaceSatisfied(t *testing.T) {
	s := newTestStore(t)
	var _ providers.LLMProvider = codexprovider.NewWithURLs(s, "http://a", "http://b")
}
