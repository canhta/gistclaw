package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/providers/providerutil"
	"github.com/canhta/gistclaw/internal/runtime"
)

const (
	defaultBaseURL   = "https://api.anthropic.com"
	defaultMaxTokens = 8096
)

// Provider calls the Anthropic Messages API.
type Provider struct {
	apiKey  string
	modelID string
	baseURL string
	client  *http.Client
}

type runStartedPayload struct {
	Objective string `json:"objective"`
}

type turnCompletedPayload struct {
	Content string `json:"content"`
}

type sessionMessagePayload struct {
	Kind string `json:"kind"`
	Body string `json:"body"`
}

type messageStream interface {
	Next() bool
	Current() anthropicsdk.MessageStreamEventUnion
	Err() error
	Close() error
}

// New creates a Provider for the given API key and default model ID.
// If the ANTHROPIC_BASE_URL environment variable is set, it is used as the
// base URL instead of the default. This allows test suites to point at an
// httptest.Server without modifying production config.
func New(apiKey, modelID string) *Provider {
	endpoint := defaultBaseURL
	if override := os.Getenv("ANTHROPIC_BASE_URL"); override != "" {
		endpoint = override
	}
	return newWithEndpoint(apiKey, modelID, endpoint)
}

// newWithEndpoint is the internal constructor that accepts a custom base URL
// (used in tests to point at httptest.Server).
func newWithEndpoint(apiKey, modelID, endpoint string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		modelID: modelID,
		baseURL: normalizeBaseURL(endpoint),
		client:  &http.Client{},
	}
}

func (p *Provider) ID() string { return "anthropic" }

// Generate sends a request to the Anthropic Messages API and returns the result.
// The StreamSink parameter is accepted for interface compliance but streaming is
// not yet implemented; the full response is buffered and returned at once.
func (p *Provider) Generate(ctx context.Context, req runtime.GenerateRequest, stream runtime.StreamSink) (runtime.GenerateResult, error) {
	modelID := req.ModelID
	if modelID == "" {
		modelID = p.modelID
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	if stream != nil {
		return p.streamMessages(ctx, req, modelID, maxTokens, stream)
	}

	client := p.sdkClient()
	apiResp, err := client.Messages.New(ctx, buildMessageParams(req, modelID, maxTokens))
	if err != nil {
		return runtime.GenerateResult{}, providerutil.ProviderError("anthropic", err)
	}

	return parseResponse(apiResp), nil
}

func (p *Provider) streamMessages(ctx context.Context, req runtime.GenerateRequest, modelID string, maxTokens int, streamSink runtime.StreamSink) (runtime.GenerateResult, error) {
	client := p.sdkClient()
	stream := client.Messages.NewStreaming(ctx, buildMessageParams(req, modelID, maxTokens))
	return consumeMessageStream(ctx, stream, streamSink)
}

func consumeMessageStream(ctx context.Context, stream messageStream, streamSink runtime.StreamSink) (runtime.GenerateResult, error) {
	defer stream.Close()
	var message anthropicsdk.Message
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return runtime.GenerateResult{}, fmt.Errorf("anthropic: accumulate stream: %w", err)
		}

		switch evt := event.AsAny().(type) {
		case anthropicsdk.ContentBlockDeltaEvent:
			if evt.Delta.Type != "text_delta" || evt.Delta.Text == "" {
				continue
			}
			if err := streamSink.OnDelta(ctx, evt.Delta.Text); err != nil {
				return runtime.GenerateResult{}, err
			}
		}
	}
	if err := stream.Err(); err != nil {
		return runtime.GenerateResult{}, providerutil.ProviderError("anthropic", err)
	}

	result := parseResponse(&message)
	if err := streamSink.OnComplete(); err != nil {
		return runtime.GenerateResult{}, err
	}
	return result, nil
}

func (p *Provider) sdkClient() anthropicsdk.Client {
	opts := []option.RequestOption{
		option.WithBaseURL(p.baseURL),
		option.WithHTTPClient(p.client),
		option.WithMaxRetries(0),
	}
	if p.apiKey != "" {
		opts = append(opts, option.WithAPIKey(p.apiKey))
	}
	return anthropicsdk.NewClient(opts...)
}

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1/messages")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	if baseURL == "" {
		return defaultBaseURL
	}
	return baseURL
}

func buildMessageParams(req runtime.GenerateRequest, modelID string, maxTokens int) anthropicsdk.MessageNewParams {
	messages := eventsToMessages(req.ConversationCtx)
	if len(messages) == 0 {
		messages = []anthropicsdk.MessageParam{
			anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("begin")),
		}
	}

	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(modelID),
		MaxTokens: int64(maxTokens),
		Messages:  messages,
	}
	if req.Instructions != "" {
		params.System = []anthropicsdk.TextBlockParam{{Text: req.Instructions}}
	}
	if len(req.ToolSpecs) > 0 {
		params.Tools = toolSpecsToAnthropicTools(req.ToolSpecs)
	}
	return params
}

// eventsToMessages converts journal events into Anthropic message pairs.
func eventsToMessages(events []model.Event) []anthropicsdk.MessageParam {
	var messages []anthropicsdk.MessageParam
	for _, ev := range events {
		switch ev.Kind {
		case "run_started":
			var payload runStartedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil || payload.Objective == "" {
				continue
			}
			messages = append(messages, anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(payload.Objective)))
		case "turn_completed":
			var payload turnCompletedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil || payload.Content == "" {
				continue
			}
			messages = append(messages, anthropicsdk.NewAssistantMessage(anthropicsdk.NewTextBlock(payload.Content)))
		case "session_message_added":
			var payload sessionMessagePayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil || payload.Body == "" {
				continue
			}
			if payload.Kind == "assistant" {
				messages = append(messages, anthropicsdk.NewAssistantMessage(anthropicsdk.NewTextBlock(payload.Body)))
				continue
			}
			messages = append(messages, anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(payload.Body)))
		case "tool_call_recorded":
			var payload providerutil.ToolCallRecordedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.ToolCallID == "" || payload.ToolName == "" {
				continue
			}
			messages = append(messages,
				anthropicsdk.NewAssistantMessage(anthropicsdk.NewToolUseBlock(payload.ToolCallID, json.RawMessage(payload.InputJSON), payload.ToolName)),
				anthropicsdk.NewUserMessage(anthropicsdk.NewToolResultBlock(payload.ToolCallID, providerutil.RenderToolResultContent(payload.OutputJSON), false)),
			)
		}
	}
	return messages
}

func toolSpecsToAnthropicTools(specs []model.ToolSpec) []anthropicsdk.ToolUnionParam {
	tools := make([]anthropicsdk.ToolUnionParam, 0, len(specs))
	for _, spec := range specs {
		tool := anthropicsdk.ToolParam{
			Name:        spec.Name,
			InputSchema: toolInputSchema(spec.InputSchemaJSON),
		}
		if spec.Description != "" {
			tool.Description = anthropicsdk.String(spec.Description)
		}
		tools = append(tools, anthropicsdk.ToolUnionParam{OfTool: &tool})
	}
	return tools
}

func toolInputSchema(raw string) anthropicsdk.ToolInputSchemaParam {
	schema := providerutil.SchemaObject(raw)
	params := anthropicsdk.ToolInputSchemaParam{}

	if properties, ok := schema["properties"]; ok {
		params.Properties = properties
		delete(schema, "properties")
	}
	if required, ok := schema["required"].([]any); ok {
		params.Required = make([]string, 0, len(required))
		for _, item := range required {
			if value, ok := item.(string); ok && value != "" {
				params.Required = append(params.Required, value)
			}
		}
	}
	delete(schema, "required")
	delete(schema, "type")
	if len(schema) > 0 {
		params.ExtraFields = schema
	}

	return params
}

func parseResponse(resp *anthropicsdk.Message) runtime.GenerateResult {
	var result runtime.GenerateResult
	if resp == nil {
		return result
	}

	result.StopReason = string(resp.StopReason)
	result.InputTokens = int(resp.Usage.InputTokens)
	result.OutputTokens = int(resp.Usage.OutputTokens)
	result.ModelID = string(resp.Model)

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
	return result
}
