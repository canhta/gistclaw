// internal/providers/codex/codex.go
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/store"
)

const (
	defaultTokenURL = "https://auth.openai.com/oauth/token"
	defaultChatURL  = "https://chatgpt.com/backend-api/conversation"

	// OAuth2 PKCE client ID for the unofficial Codex backend.
	codexClientID = "pdlLIX2Y72MIl2rhLhTE9VV9bN905kBh"
	codexAudience = "https://api.openai.com/v1"
)

// tokenRefreshBuffer is how early we treat a token as expired.
// 5 minutes provides a safety margin for slow VPS networks.
const tokenRefreshBuffer = 5 * time.Minute

// StoredToken is the JSON structure persisted in store.provider_credentials for "codex".
// Exported so tests can construct and inspect it.
type StoredToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Provider implements providers.LLMProvider using the unofficial chatgpt.com backend
// with OAuth2 PKCE authentication.
//
// TotalCostUSD is always 0 — billing on the unofficial backend is opaque.
type Provider struct {
	mu       sync.Mutex
	store    *store.Store
	token    *StoredToken
	tokenURL string
	chatURL  string
}

// New creates a Codex provider with default OAuth2 and backend URLs.
func New(s *store.Store) *Provider {
	return NewWithURLs(s, defaultTokenURL, defaultChatURL)
}

// NewWithURLs creates a Codex provider with custom URLs (for testing).
func NewWithURLs(s *store.Store, tokenURL, chatURL string) *Provider {
	return &Provider{store: s, tokenURL: tokenURL, chatURL: chatURL}
}

// Name implements providers.LLMProvider.
func (p *Provider) Name() string { return "codex" }

// Chat implements providers.LLMProvider.
// It ensures a valid access token is available (loading from store, refreshing if
// expired, or triggering a PKCE CLI flow if no token exists), then calls the
// unofficial chatgpt.com backend.
func (p *Provider) Chat(ctx context.Context, messages []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	token, err := p.ensureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("codex: ensure token: %w", err)
	}

	body, err := buildChatRequest(messages)
	if err != nil {
		return nil, fmt.Errorf("codex: build request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.chatURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("codex: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codex: do request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("codex: backend returned HTTP %d", httpResp.StatusCode)
	}

	content, err := parseChatResponse(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("codex: parse response: %w", err)
	}

	return &providers.LLMResponse{
		Content:  content,
		ToolCall: nil, // unofficial backend does not support tool calls
		Usage: providers.Usage{
			TotalCostUSD: 0, // billing opaque on unofficial backend
		},
	}, nil
}

// ensureToken returns a valid token, refreshing or re-authenticating as needed.
func (p *Provider) ensureToken(ctx context.Context) (*StoredToken, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try in-memory first.
	if p.token != nil && time.Now().Before(p.token.ExpiresAt.Add(-tokenRefreshBuffer)) {
		return p.token, nil
	}

	// Try loading from store.
	stored, err := p.store.GetProviderCredentials("codex")
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("load credentials: %w", err)
	}

	var tok StoredToken
	if stored != "" {
		if err := json.Unmarshal([]byte(stored), &tok); err != nil {
			log.Warn().Err(err).Msg("codex: failed to parse stored token; will re-authenticate")
		} else if time.Now().Before(tok.ExpiresAt.Add(-tokenRefreshBuffer)) {
			// Token still valid.
			p.token = &tok
			return p.token, nil
		} else if tok.RefreshToken != "" {
			// Token expired but we have a refresh token.
			refreshed, err := p.refreshToken(ctx, tok.RefreshToken)
			if err != nil {
				log.Warn().Err(err).Msg("codex: token refresh failed; will re-authenticate")
			} else {
				p.token = refreshed
				if err := p.persistToken(refreshed); err != nil {
					log.Error().Err(err).Msg("codex: failed to persist refreshed token")
				}
				return p.token, nil
			}
		}
	}

	// No valid token — trigger PKCE CLI flow.
	newTok, err := p.runPKCEFlow(ctx)
	if err != nil {
		return nil, fmt.Errorf("PKCE flow: %w", err)
	}
	p.token = newTok
	if err := p.persistToken(newTok); err != nil {
		log.Error().Err(err).Msg("codex: failed to persist new token")
	}
	return p.token, nil
}

// refreshToken exchanges a refresh_token for a new access token using the token endpoint.
func (p *Provider) refreshToken(ctx context.Context, refreshToken string) (*StoredToken, error) {
	cfg := &oauth2.Config{
		ClientID: codexClientID,
		Endpoint: oauth2.Endpoint{TokenURL: p.tokenURL},
	}
	ts := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	oaToken, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("refresh: %w", err)
	}
	return &StoredToken{
		AccessToken:  oaToken.AccessToken,
		RefreshToken: oaToken.RefreshToken,
		ExpiresAt:    oaToken.Expiry,
	}, nil
}

// runPKCEFlow performs the OAuth2 PKCE CLI flow:
// 1. Generate code verifier + challenge.
// 2. Print authorization URL.
// 3. Read authorization code from stdin.
// 4. Exchange code for tokens.
func (p *Provider) runPKCEFlow(ctx context.Context) (*StoredToken, error) {
	// Generate PKCE code verifier (43-128 chars of random URL-safe base64).
	verifier := oauth2.GenerateVerifier()

	cfg := &oauth2.Config{
		ClientID: codexClientID,
		Scopes:   []string{"openid", "profile", "email", "offline_access"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://auth.openai.com/authorize",
			TokenURL: p.tokenURL,
		},
		RedirectURL: "http://localhost:8085/callback",
	}

	authURL := cfg.AuthCodeURL("state",
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("audience", codexAudience),
	)

	fmt.Printf("\n=== Codex OAuth Setup ===\n")
	fmt.Printf("Open the following URL in your browser:\n\n  %s\n\n", authURL)
	fmt.Printf("Paste the authorization code here: ")

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("read auth code: %w", err)
	}

	oaToken, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	return &StoredToken{
		AccessToken:  oaToken.AccessToken,
		RefreshToken: oaToken.RefreshToken,
		ExpiresAt:    oaToken.Expiry,
	}, nil
}

// persistToken saves the token to the store as JSON.
func (p *Provider) persistToken(tok *StoredToken) error {
	data, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	return p.store.SetProviderCredentials("codex", string(data))
}

// buildChatRequest serialises the messages into the chatgpt.com /backend-api/conversation body.
func buildChatRequest(messages []providers.Message) ([]byte, error) {
	type msgContent struct {
		ContentType string   `json:"content_type"`
		Parts       []string `json:"parts"`
	}
	type msgAuthor struct {
		Role string `json:"role"`
	}
	type msg struct {
		ID      string     `json:"id"`
		Author  msgAuthor  `json:"author"`
		Content msgContent `json:"content"`
	}
	type reqBody struct {
		Action          string `json:"action"`
		Messages        []msg  `json:"messages"`
		ParentMessageID string `json:"parent_message_id"`
		Model           string `json:"model"`
	}

	var msgs []msg
	for _, m := range messages {
		var role string
		switch m.Role {
		case "user":
			role = "user"
		case "assistant":
			role = "assistant"
		case "system":
			role = "system"
		default:
			continue // skip tool messages — unofficial backend does not support them
		}
		msgs = append(msgs, msg{
			ID:     fmt.Sprintf("msg-%d", len(msgs)),
			Author: msgAuthor{Role: role},
			Content: msgContent{
				ContentType: "text",
				Parts:       []string{m.Content},
			},
		})
	}

	body := reqBody{
		Action:          "next",
		Messages:        msgs,
		ParentMessageID: "00000000-0000-0000-0000-000000000000",
		Model:           "text-davinci-002-render-sha", // free tier model on chatgpt.com
	}
	return json.Marshal(body)
}

// parseChatResponse extracts the assistant's text content from the backend response.
// The unofficial backend returns a JSON object with message.content.parts.
func parseChatResponse(body io.Reader) (string, error) {
	var resp struct {
		Message struct {
			Content struct {
				Parts []string `json:"parts"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(resp.Message.Content.Parts) == 0 {
		return "", errors.New("no content parts in response")
	}
	return resp.Message.Content.Parts[0], nil
}
