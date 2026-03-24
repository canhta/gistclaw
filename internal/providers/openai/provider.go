// Package openai implements a runtime.Provider that calls any OpenAI-compatible
// Chat Completions API (/v1/chat/completions). This covers OpenAI, Azure OpenAI,
// Groq, Together AI, Ollama, LM Studio, and any other compatible endpoint.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

const defaultEndpoint = "https://api.openai.com"

// Provider calls any OpenAI-compatible Chat Completions API.
// Set baseURL to a custom endpoint (e.g. "http://localhost:11434" for Ollama).
type Provider struct {
	apiKey  string
	modelID string
	baseURL string
	client  *http.Client
}

// New creates a Provider. baseURL overrides the default OpenAI endpoint when
// non-empty, enabling use with compatible providers (Ollama, Groq, Together AI,
// LM Studio, Azure OpenAI, etc.).
func New(apiKey, modelID, baseURL string) *Provider {
	if baseURL == "" {
		baseURL = defaultEndpoint
	}
	return &Provider{
		apiKey:  apiKey,
		modelID: modelID,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

func (p *Provider) ID() string { return "openai" }

// Generate sends a request to the OpenAI-compatible Chat Completions endpoint.
// StreamSink is accepted for interface compliance but streaming is not yet
// implemented — the full response is buffered.
func (p *Provider) Generate(ctx context.Context, req runtime.GenerateRequest, _ runtime.StreamSink) (runtime.GenerateResult, error) {
	modelID := req.ModelID
	if modelID == "" {
		modelID = p.modelID
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8096
	}

	var msgs []chatMessage
	if req.Instructions != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.Instructions})
	}
	msgs = append(msgs, eventsToMessages(req.ConversationCtx)...)
	// Ensure at least one non-system message so the API accepts the request.
	if len(msgs) == 0 || (len(msgs) == 1 && msgs[0].Role == "system") {
		msgs = append(msgs, chatMessage{Role: "user", Content: "begin"})
	}

	body := map[string]any{
		"model":      modelID,
		"max_tokens": maxTokens,
		"messages":   msgs,
	}
	if len(req.ToolSpecs) > 0 {
		body["tools"] = toolSpecsToOpenAITools(req.ToolSpecs)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return runtime.GenerateResult{}, fmt.Errorf("openai: marshal request: %w", err)
	}

	endpoint := p.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return runtime.GenerateResult{}, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return runtime.GenerateResult{}, fmt.Errorf("openai: http: %w", err)
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

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAITool struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type apiResponse struct {
	Choices []choice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type choice struct {
	Message      choiceMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type choiceMessage struct {
	Role      string     `json:"role"`
	Content   *string    `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ── Conversion helpers ──────────────────────────────────────────────────────────

func eventsToMessages(events []model.Event) []chatMessage {
	var msgs []chatMessage
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
				msgs = append(msgs, chatMessage{Role: "user", Content: payload.Objective})
			}
		case "turn_completed":
			var payload struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.Content != "" {
				msgs = append(msgs, chatMessage{Role: "assistant", Content: payload.Content})
			}
		}
	}
	return msgs
}

func toolSpecsToOpenAITools(specs []model.ToolSpec) []openAITool {
	tools := make([]openAITool, 0, len(specs))
	for _, s := range specs {
		params := json.RawMessage(s.InputSchemaJSON)
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, openAITool{
			Type: "function",
			Function: toolFunction{
				Name:        s.Name,
				Description: s.Description,
				Parameters:  params,
			},
		})
	}
	return tools
}

func parseResponse(resp apiResponse) (runtime.GenerateResult, error) {
	var result runtime.GenerateResult
	result.InputTokens = resp.Usage.PromptTokens
	result.OutputTokens = resp.Usage.CompletionTokens

	if len(resp.Choices) == 0 {
		return result, nil
	}

	choice := resp.Choices[0]
	result.StopReason = choice.FinishReason
	if choice.Message.Content != nil {
		result.Content = *choice.Message.Content
	}

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, model.ToolCallRequest{
			ID:        tc.ID,
			ToolName:  tc.Function.Name,
			InputJSON: []byte(tc.Function.Arguments),
		})
	}
	return result, nil
}
