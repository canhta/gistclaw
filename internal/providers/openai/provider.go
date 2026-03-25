// Package openai implements a runtime.Provider that calls OpenAI-compatible
// Chat Completions or Responses APIs. This covers OpenAI, Azure OpenAI, Groq,
// Together AI, Ollama, LM Studio, and other compatible endpoints.
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

const (
	defaultEndpoint        = "https://api.openai.com"
	defaultMaxTokens       = 8096
	wireAPIChatCompletions = "chat_completions"
	wireAPIResponses       = "responses"
)

// Provider calls any OpenAI-compatible Chat Completions or Responses API.
// Set baseURL to a custom endpoint (e.g. "http://localhost:11434" for Ollama).
type Provider struct {
	apiKey  string
	modelID string
	baseURL string
	wireAPI string
	client  *http.Client
}

// New creates a Provider. baseURL overrides the default OpenAI endpoint when
// non-empty, enabling use with compatible providers (Ollama, Groq, Together AI,
// LM Studio, Azure OpenAI, etc.). wireAPI selects the HTTP surface:
// "chat_completions" (default) or "responses".
func New(apiKey, modelID, baseURL, wireAPI string) *Provider {
	if baseURL == "" {
		baseURL = defaultEndpoint
	}
	if wireAPI == "" {
		wireAPI = wireAPIChatCompletions
	}
	return &Provider{
		apiKey:  apiKey,
		modelID: modelID,
		baseURL: strings.TrimRight(baseURL, "/"),
		wireAPI: wireAPI,
		client:  &http.Client{},
	}
}

func (p *Provider) ID() string { return "openai" }

func (p *Provider) Generate(ctx context.Context, req runtime.GenerateRequest, _ runtime.StreamSink) (runtime.GenerateResult, error) {
	switch p.wireAPI {
	case wireAPIResponses:
		return p.generateResponses(ctx, req)
	default:
		return p.generateChatCompletions(ctx, req)
	}
}

func (p *Provider) generateChatCompletions(ctx context.Context, req runtime.GenerateRequest) (runtime.GenerateResult, error) {
	modelID := req.ModelID
	if modelID == "" {
		modelID = p.modelID
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	var msgs []chatMessage
	if req.Instructions != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.Instructions})
	}
	msgs = append(msgs, eventsToChatMessages(req.ConversationCtx)...)
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

	endpoint := p.endpoint("/chat/completions")
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

	var apiResp chatCompletionsAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return runtime.GenerateResult{}, &model.ProviderError{
			Code:      model.ErrMalformedResponse,
			Message:   fmt.Sprintf("decode response (status %d): %v", resp.StatusCode, err),
			Retryable: false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		return runtime.GenerateResult{}, apiError(resp.StatusCode, apiResp.Error)
	}

	return parseChatCompletionsResponse(apiResp)
}

func (p *Provider) generateResponses(ctx context.Context, req runtime.GenerateRequest) (runtime.GenerateResult, error) {
	modelID := req.ModelID
	if modelID == "" {
		modelID = p.modelID
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	input := eventsToResponseInput(req.ConversationCtx)
	if len(input) == 0 {
		input = []any{
			responseInputMessage{
				Role: "user",
				Content: []responseInputText{
					{Type: "input_text", Text: "begin"},
				},
			},
		}
	}

	body := map[string]any{
		"model":             modelID,
		"input":             input,
		"max_output_tokens": maxTokens,
	}
	if req.Instructions != "" {
		body["instructions"] = req.Instructions
	}
	if len(req.ToolSpecs) > 0 {
		body["tool_choice"] = "auto"
		body["tools"] = toolSpecsToResponseTools(req.ToolSpecs)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return runtime.GenerateResult{}, fmt.Errorf("openai: marshal request: %w", err)
	}

	endpoint := p.endpoint("/responses")
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

	var apiResp responsesAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return runtime.GenerateResult{}, &model.ProviderError{
			Code:      model.ErrMalformedResponse,
			Message:   fmt.Sprintf("decode response (status %d): %v", resp.StatusCode, err),
			Retryable: false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		return runtime.GenerateResult{}, apiError(resp.StatusCode, apiResp.Error)
	}

	return parseResponsesResponse(apiResp), nil
}

func (p *Provider) endpoint(path string) string {
	if strings.HasSuffix(p.baseURL, "/v1") {
		return p.baseURL + path
	}
	return p.baseURL + "/v1" + path
}

// ── Internal types ─────────────────────────────────────────────────────────────

type chatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
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

type responseTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type providerErrorPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type chatCompletionsAPIResponse struct {
	Choices []choice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error providerErrorPayload `json:"error"`
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

type responseInputMessage struct {
	Role    string              `json:"role"`
	Content []responseInputText `json:"content"`
}

type responseInputText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesAPIResponse struct {
	Output []responseOutput `json:"output"`
	Usage  struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error providerErrorPayload `json:"error"`
}

type responseOutput struct {
	Type      string                  `json:"type"`
	ID        string                  `json:"id"`
	CallID    string                  `json:"call_id"`
	Name      string                  `json:"name"`
	Arguments string                  `json:"arguments"`
	Content   []responseOutputContent `json:"content"`
}

type responseOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ── Conversion helpers ──────────────────────────────────────────────────────────

type toolCallRecordedPayload struct {
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	InputJSON  json.RawMessage `json:"input_json"`
	OutputJSON json.RawMessage `json:"output_json"`
	Decision   string          `json:"decision"`
}

func eventsToChatMessages(events []model.Event) []chatMessage {
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
		case "session_message_added":
			var payload struct {
				Kind string `json:"kind"`
				Body string `json:"body"`
			}
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.Body == "" {
				continue
			}
			role := "user"
			if payload.Kind == "assistant" {
				role = "assistant"
			}
			msgs = append(msgs, chatMessage{Role: role, Content: payload.Body})
		case "tool_call_recorded":
			var payload toolCallRecordedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.ToolCallID == "" || payload.ToolName == "" {
				continue
			}
			var tc toolCall
			tc.ID = payload.ToolCallID
			tc.Type = "function"
			tc.Function.Name = payload.ToolName
			tc.Function.Arguments = string(payload.InputJSON)
			msgs = append(msgs,
				chatMessage{Role: "assistant", ToolCalls: []toolCall{tc}},
				chatMessage{
					Role:       "tool",
					ToolCallID: payload.ToolCallID,
					Content:    renderToolResultContent(payload.OutputJSON),
				},
			)
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

func toolSpecsToResponseTools(specs []model.ToolSpec) []responseTool {
	tools := make([]responseTool, 0, len(specs))
	for _, s := range specs {
		params := json.RawMessage(s.InputSchemaJSON)
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, responseTool{
			Type:        "function",
			Name:        s.Name,
			Description: s.Description,
			Parameters:  params,
		})
	}
	return tools
}

func eventsToResponseInput(events []model.Event) []any {
	input := make([]any, 0, len(events))
	for _, ev := range events {
		switch ev.Kind {
		case "run_started":
			var payload struct {
				Objective string `json:"objective"`
			}
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.Objective == "" {
				continue
			}
			input = append(input, responseInputMessage{
				Role: "user",
				Content: []responseInputText{
					{Type: "input_text", Text: payload.Objective},
				},
			})
		case "turn_completed":
			var payload struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.Content == "" {
				continue
			}
			input = append(input, responseInputMessage{
				Role: "assistant",
				Content: []responseInputText{
					{Type: "input_text", Text: payload.Content},
				},
			})
		case "session_message_added":
			var payload struct {
				Kind string `json:"kind"`
				Body string `json:"body"`
			}
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.Body == "" {
				continue
			}
			role := "user"
			if payload.Kind == "assistant" {
				role = "assistant"
			}
			input = append(input, responseInputMessage{
				Role: role,
				Content: []responseInputText{
					{Type: "input_text", Text: payload.Body},
				},
			})
		case "tool_call_recorded":
			var payload toolCallRecordedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.ToolCallID == "" || payload.ToolName == "" {
				continue
			}
			input = append(input,
				map[string]any{
					"type":      "function_call",
					"call_id":   payload.ToolCallID,
					"name":      payload.ToolName,
					"arguments": string(payload.InputJSON),
				},
				map[string]any{
					"type":    "function_call_output",
					"call_id": payload.ToolCallID,
					"output":  renderToolResultContent(payload.OutputJSON),
				},
			)
		}
	}
	return input
}

func renderToolResultContent(raw json.RawMessage) string {
	var result model.ToolResult
	if err := json.Unmarshal(raw, &result); err == nil {
		switch {
		case result.Output != "" && result.Error != "":
			return result.Output + "\n" + result.Error
		case result.Output != "":
			return result.Output
		case result.Error != "":
			return result.Error
		}
	}
	if len(raw) == 0 {
		return ""
	}
	return string(raw)
}

func parseChatCompletionsResponse(resp chatCompletionsAPIResponse) (runtime.GenerateResult, error) {
	var result runtime.GenerateResult
	result.InputTokens = resp.Usage.PromptTokens
	result.OutputTokens = resp.Usage.CompletionTokens

	if len(resp.Choices) == 0 {
		return result, nil
	}

	choice := resp.Choices[0]
	result.StopReason = choice.FinishReason
	if result.StopReason == "stop" {
		result.StopReason = "end_turn"
	}
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

func parseResponsesResponse(resp responsesAPIResponse) runtime.GenerateResult {
	result := runtime.GenerateResult{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		StopReason:   "end_turn",
	}

	var textParts []string
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					textParts = append(textParts, content.Text)
				}
			}
		case "function_call":
			id := item.CallID
			if id == "" {
				id = item.ID
			}
			result.ToolCalls = append(result.ToolCalls, model.ToolCallRequest{
				ID:        id,
				ToolName:  item.Name,
				InputJSON: []byte(item.Arguments),
			})
		}
	}
	result.Content = strings.Join(textParts, "\n")
	if len(result.ToolCalls) > 0 {
		result.StopReason = "tool_calls"
	}
	return result
}

func apiError(statusCode int, payload providerErrorPayload) error {
	errCode := model.ProviderErrorCode(payload.Type)
	if errCode == "" {
		errCode = model.ErrMalformedResponse
	}
	return &model.ProviderError{
		Code:      errCode,
		Message:   payload.Message,
		Retryable: statusCode == http.StatusTooManyRequests || statusCode >= 500,
	}
}
