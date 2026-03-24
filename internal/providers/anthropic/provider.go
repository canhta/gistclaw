package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

const (
	defaultEndpoint   = "https://api.anthropic.com/v1/messages"
	anthropicVersion  = "2023-06-01"
	defaultMaxTokens  = 8096
)

// Provider calls the Anthropic Messages API using stdlib net/http.
type Provider struct {
	apiKey   string
	modelID  string
	endpoint string
	client   *http.Client
}

// New creates a Provider for the given API key and default model ID.
// If the ANTHROPIC_BASE_URL environment variable is set, it is used as the
// base URL instead of the default. This allows test suites to point at an
// httptest.Server without modifying production config.
func New(apiKey, modelID string) *Provider {
	endpoint := defaultEndpoint
	if override := os.Getenv("ANTHROPIC_BASE_URL"); override != "" {
		endpoint = override
	}
	return newWithEndpoint(apiKey, modelID, endpoint)
}

// newWithEndpoint is the internal constructor that accepts a custom endpoint
// (used in tests to point at httptest.Server).
func newWithEndpoint(apiKey, modelID, endpoint string) *Provider {
	return &Provider{
		apiKey:   apiKey,
		modelID:  modelID,
		endpoint: endpoint,
		client:   &http.Client{},
	}
}

func (p *Provider) ID() string { return "anthropic" }

// Generate sends a request to the Anthropic Messages API and returns the result.
// The StreamSink parameter is accepted for interface compliance but streaming is
// not yet implemented — the full response is buffered and returned at once.
func (p *Provider) Generate(ctx context.Context, req runtime.GenerateRequest, _ runtime.StreamSink) (runtime.GenerateResult, error) {
	modelID := req.ModelID
	if modelID == "" {
		modelID = p.modelID
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	body := map[string]any{
		"model":      modelID,
		"max_tokens": maxTokens,
	}
	if req.Instructions != "" {
		body["system"] = req.Instructions
	}

	msgs := eventsToMessages(req.ConversationCtx)
	if len(msgs) == 0 {
		// Anthropic requires at least one user message. Inject a minimal
		// placeholder so the API call is valid even when no history exists.
		msgs = []message{{Role: "user", Content: "begin"}}
	}
	body["messages"] = msgs

	if len(req.ToolSpecs) > 0 {
		body["tools"] = toolSpecsToAnthropicTools(req.ToolSpecs)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return runtime.GenerateResult{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return runtime.GenerateResult{}, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return runtime.GenerateResult{}, fmt.Errorf("anthropic: http: %w", err)
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return runtime.GenerateResult{}, &model.ProviderError{
			Code:      model.ErrMalformedResponse,
			Message:   fmt.Sprintf("decode response (status %d): %v", resp.StatusCode, err),
			Retryable: false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		errCode := model.ProviderErrorCode(apiResp.Error.Type)
		if errCode == "" {
			errCode = model.ErrMalformedResponse
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return runtime.GenerateResult{}, &model.ProviderError{
			Code:      errCode,
			Message:   apiResp.Error.Message,
			Retryable: retryable,
		}
	}

	return parseResponse(apiResp)
}

// ── Internal types ─────────────────────────────────────────────────────────────

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type apiResponse struct {
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ── Conversion helpers ──────────────────────────────────────────────────────────

// eventsToMessages converts journal events into Anthropic message pairs.
// Only run_started (→ user) and turn_completed (→ assistant) events contribute.
func eventsToMessages(events []model.Event) []message {
	var msgs []message
	for _, ev := range events {
		switch ev.Kind {
		case "run_started":
			var payload struct {
				Objective string `json:"objective"`
			}
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.Objective != "" {
				msgs = append(msgs, message{Role: "user", Content: payload.Objective})
			}
		case "turn_completed":
			var payload struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.Content != "" {
				msgs = append(msgs, message{Role: "assistant", Content: payload.Content})
			}
		}
	}
	return msgs
}

func toolSpecsToAnthropicTools(specs []model.ToolSpec) []anthropicTool {
	tools := make([]anthropicTool, 0, len(specs))
	for _, s := range specs {
		schema := json.RawMessage(s.InputSchemaJSON)
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, anthropicTool{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: schema,
		})
	}
	return tools
}

func parseResponse(resp apiResponse) (runtime.GenerateResult, error) {
	var result runtime.GenerateResult
	result.StopReason = resp.StopReason
	result.InputTokens = resp.Usage.InputTokens
	result.OutputTokens = resp.Usage.OutputTokens

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			inputJSON, _ := json.Marshal(block.Input)
			result.ToolCalls = append(result.ToolCalls, model.ToolCallRequest{
				ID:        block.ID,
				ToolName:  block.Name,
				InputJSON: inputJSON,
			})
		}
	}
	return result, nil
}
