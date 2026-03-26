// Package openai implements a runtime.Provider that calls OpenAI-compatible
// Chat Completions or Responses APIs. This covers OpenAI, Azure OpenAI, Groq,
// Together AI, Ollama, LM Studio, and other compatible endpoints.
package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/providers/providerutil"
	"github.com/canhta/gistclaw/internal/runtime"
	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	openairesponses "github.com/openai/openai-go/responses"
)

const (
	defaultBaseURL         = "https://api.openai.com"
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

// New creates a Provider. baseURL overrides the default OpenAI endpoint when
// non-empty, enabling use with compatible providers (Ollama, Groq, Together AI,
// LM Studio, Azure OpenAI, etc.). wireAPI selects the HTTP surface:
// "chat_completions" (default) or "responses".
func New(apiKey, modelID, baseURL, wireAPI string) *Provider {
	if wireAPI == "" {
		wireAPI = wireAPIChatCompletions
	}
	return &Provider{
		apiKey:  apiKey,
		modelID: modelID,
		baseURL: normalizeBaseURL(baseURL),
		wireAPI: wireAPI,
		client:  &http.Client{},
	}
}

func (p *Provider) ID() string { return "openai" }

func (p *Provider) Generate(ctx context.Context, req runtime.GenerateRequest, stream runtime.StreamSink) (runtime.GenerateResult, error) {
	if stream != nil {
		switch p.wireAPI {
		case wireAPIResponses:
			return p.streamResponses(ctx, req, stream)
		default:
			return p.streamChatCompletions(ctx, req, stream)
		}
	}

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

	client := p.sdkClient()
	apiResp, err := client.Chat.Completions.New(ctx, buildChatCompletionParams(req, modelID, maxTokens))
	if err != nil {
		return runtime.GenerateResult{}, providerutil.ProviderError("openai", err)
	}

	return parseChatCompletionsResponse(apiResp), nil
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

	client := p.sdkClient()
	apiResp, err := client.Responses.New(ctx, buildResponsesParams(req, modelID, maxTokens))
	if err != nil {
		return runtime.GenerateResult{}, providerutil.ProviderError("openai", err)
	}

	return parseResponsesResponse(apiResp), nil
}

func (p *Provider) streamChatCompletions(ctx context.Context, req runtime.GenerateRequest, streamSink runtime.StreamSink) (runtime.GenerateResult, error) {
	modelID := req.ModelID
	if modelID == "" {
		modelID = p.modelID
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	client := p.sdkClient()
	stream := client.Chat.Completions.NewStreaming(ctx, buildChatCompletionParams(req, modelID, maxTokens))
	defer stream.Close()

	var acc openaisdk.ChatCompletionAccumulator
	for stream.Next() {
		chunk := stream.Current()
		if !acc.AddChunk(chunk) {
			return runtime.GenerateResult{}, &model.ProviderError{
				Code:    model.ErrMalformedResponse,
				Message: "openai: invalid streaming chat completion chunk sequence",
			}
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			if err := streamSink.OnDelta(ctx, choice.Delta.Content); err != nil {
				return runtime.GenerateResult{}, err
			}
		}
	}
	if err := stream.Err(); err != nil {
		return runtime.GenerateResult{}, providerutil.ProviderError("openai", err)
	}

	result := parseChatCompletionsResponse(&acc.ChatCompletion)
	if err := streamSink.OnComplete(); err != nil {
		return runtime.GenerateResult{}, err
	}
	return result, nil
}

func (p *Provider) streamResponses(ctx context.Context, req runtime.GenerateRequest, streamSink runtime.StreamSink) (runtime.GenerateResult, error) {
	modelID := req.ModelID
	if modelID == "" {
		modelID = p.modelID
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	client := p.sdkClient()
	stream := client.Responses.NewStreaming(ctx, buildResponsesParams(req, modelID, maxTokens))
	defer stream.Close()

	var completed *openairesponses.Response
	for stream.Next() {
		event := stream.Current()
		switch evt := event.AsAny().(type) {
		case openairesponses.ResponseTextDeltaEvent:
			if evt.Delta == "" {
				continue
			}
			if err := streamSink.OnDelta(ctx, evt.Delta); err != nil {
				return runtime.GenerateResult{}, err
			}
		case openairesponses.ResponseCompletedEvent:
			response := evt.Response
			completed = &response
		}
	}
	if err := stream.Err(); err != nil {
		return runtime.GenerateResult{}, providerutil.ProviderError("openai", err)
	}
	if completed == nil {
		return runtime.GenerateResult{}, &model.ProviderError{
			Code:    model.ErrMalformedResponse,
			Message: "openai: streaming response completed without a final response payload",
		}
	}

	result := parseResponsesResponse(completed)
	if err := streamSink.OnComplete(); err != nil {
		return runtime.GenerateResult{}, err
	}
	return result, nil
}

func (p *Provider) sdkClient() openaisdk.Client {
	opts := []option.RequestOption{
		option.WithBaseURL(p.baseURL + "/v1/"),
		option.WithHTTPClient(p.client),
		option.WithMaxRetries(0),
	}
	if p.apiKey != "" {
		opts = append(opts, option.WithAPIKey(p.apiKey))
	}
	return openaisdk.NewClient(opts...)
}

func normalizeBaseURL(baseURL string) string {
	if baseURL == "" {
		return defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1/chat/completions")
	baseURL = strings.TrimSuffix(baseURL, "/v1/responses")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	if baseURL == "" {
		return defaultBaseURL
	}
	return baseURL
}

func buildChatCompletionParams(req runtime.GenerateRequest, modelID string, maxTokens int) openaisdk.ChatCompletionNewParams {
	messages := eventsToChatMessages(req.ConversationCtx)
	if req.Instructions != "" {
		messages = append([]openaisdk.ChatCompletionMessageParamUnion{openaisdk.SystemMessage(req.Instructions)}, messages...)
	}
	if len(messages) == 0 || (len(messages) == 1 && messages[0].OfSystem != nil) {
		messages = append(messages, openaisdk.UserMessage("begin"))
	}

	params := openaisdk.ChatCompletionNewParams{
		Model:     openaisdk.ChatModel(modelID),
		MaxTokens: openaisdk.Int(int64(maxTokens)),
		Messages:  messages,
	}
	if len(req.ToolSpecs) > 0 {
		params.Tools = toolSpecsToOpenAITools(req.ToolSpecs)
	}
	return params
}

func buildResponsesParams(req runtime.GenerateRequest, modelID string, maxTokens int) openairesponses.ResponseNewParams {
	input := eventsToResponseInput(req.ConversationCtx)
	if len(input) == 0 {
		input = openairesponses.ResponseInputParam{
			openairesponses.ResponseInputItemParamOfMessage("begin", openairesponses.EasyInputMessageRoleUser),
		}
	}

	params := openairesponses.ResponseNewParams{
		Model:           openaisdk.ResponsesModel(modelID),
		MaxOutputTokens: openaisdk.Int(int64(maxTokens)),
		Input: openairesponses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
	}
	if req.Instructions != "" {
		params.Instructions = openaisdk.String(req.Instructions)
	}
	if len(req.ToolSpecs) > 0 {
		params.ToolChoice = openairesponses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: openaisdk.Opt(openairesponses.ToolChoiceOptionsAuto),
		}
		params.Tools = toolSpecsToResponseTools(req.ToolSpecs)
	}
	return params
}

func eventsToChatMessages(events []model.Event) []openaisdk.ChatCompletionMessageParamUnion {
	var messages []openaisdk.ChatCompletionMessageParamUnion
	for _, ev := range events {
		switch ev.Kind {
		case "run_started":
			var payload runStartedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil || payload.Objective == "" {
				continue
			}
			messages = append(messages, openaisdk.UserMessage(payload.Objective))
		case "turn_completed":
			var payload turnCompletedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil || payload.Content == "" {
				continue
			}
			messages = append(messages, openaisdk.ChatCompletionMessageParamOfAssistant(payload.Content))
		case "session_message_added":
			var payload sessionMessagePayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil || payload.Body == "" {
				continue
			}
			if payload.Kind == "assistant" {
				messages = append(messages, openaisdk.ChatCompletionMessageParamOfAssistant(payload.Body))
				continue
			}
			messages = append(messages, openaisdk.UserMessage(payload.Body))
		case "tool_call_recorded":
			var payload providerutil.ToolCallRecordedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.ToolCallID == "" || payload.ToolName == "" {
				continue
			}

			assistant := openaisdk.ChatCompletionAssistantMessageParam{
				ToolCalls: []openaisdk.ChatCompletionMessageToolCallParam{
					{
						ID: payload.ToolCallID,
						Function: openaisdk.ChatCompletionMessageToolCallFunctionParam{
							Name:      payload.ToolName,
							Arguments: string(payload.InputJSON),
						},
					},
				},
			}
			messages = append(messages,
				openaisdk.ChatCompletionMessageParamUnion{OfAssistant: &assistant},
				openaisdk.ToolMessage(providerutil.RenderToolResultContent(payload.OutputJSON), payload.ToolCallID),
			)
		}
	}
	return messages
}

func eventsToResponseInput(events []model.Event) openairesponses.ResponseInputParam {
	input := make(openairesponses.ResponseInputParam, 0, len(events))
	for _, ev := range events {
		switch ev.Kind {
		case "run_started":
			var payload runStartedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil || payload.Objective == "" {
				continue
			}
			input = append(input, openairesponses.ResponseInputItemParamOfMessage(payload.Objective, openairesponses.EasyInputMessageRoleUser))
		case "turn_completed":
			var payload turnCompletedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil || payload.Content == "" {
				continue
			}
			input = append(input, openairesponses.ResponseInputItemParamOfMessage(payload.Content, openairesponses.EasyInputMessageRoleAssistant))
		case "session_message_added":
			var payload sessionMessagePayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil || payload.Body == "" {
				continue
			}
			role := openairesponses.EasyInputMessageRoleUser
			if payload.Kind == "assistant" {
				role = openairesponses.EasyInputMessageRoleAssistant
			}
			input = append(input, openairesponses.ResponseInputItemParamOfMessage(payload.Body, role))
		case "tool_call_recorded":
			var payload providerutil.ToolCallRecordedPayload
			if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
				continue
			}
			if payload.ToolCallID == "" || payload.ToolName == "" {
				continue
			}
			input = append(input,
				openairesponses.ResponseInputItemParamOfFunctionCall(string(payload.InputJSON), payload.ToolCallID, payload.ToolName),
				openairesponses.ResponseInputItemParamOfFunctionCallOutput(payload.ToolCallID, providerutil.RenderToolResultContent(payload.OutputJSON)),
			)
		}
	}
	return input
}

func toolSpecsToOpenAITools(specs []model.ToolSpec) []openaisdk.ChatCompletionToolParam {
	tools := make([]openaisdk.ChatCompletionToolParam, 0, len(specs))
	for _, spec := range specs {
		function := openaisdk.FunctionDefinitionParam{
			Name:       spec.Name,
			Parameters: openaisdk.FunctionParameters(providerutil.SchemaObject(spec.InputSchemaJSON)),
		}
		if spec.Description != "" {
			function.Description = openaisdk.String(spec.Description)
		}
		tools = append(tools, openaisdk.ChatCompletionToolParam{Function: function})
	}
	return tools
}

func toolSpecsToResponseTools(specs []model.ToolSpec) []openairesponses.ToolUnionParam {
	tools := make([]openairesponses.ToolUnionParam, 0, len(specs))
	for _, spec := range specs {
		tool := openairesponses.FunctionToolParam{
			Name:       spec.Name,
			Strict:     openaisdk.Bool(true),
			Parameters: providerutil.SchemaObject(spec.InputSchemaJSON),
		}
		if spec.Description != "" {
			tool.Description = openaisdk.String(spec.Description)
		}
		tools = append(tools, openairesponses.ToolUnionParam{OfFunction: &tool})
	}
	return tools
}

func parseChatCompletionsResponse(resp *openaisdk.ChatCompletion) runtime.GenerateResult {
	var result runtime.GenerateResult
	if resp == nil {
		return result
	}

	result.InputTokens = int(resp.Usage.PromptTokens)
	result.OutputTokens = int(resp.Usage.CompletionTokens)
	result.ModelID = resp.Model
	if len(resp.Choices) == 0 {
		return result
	}

	choice := resp.Choices[0]
	result.StopReason = choice.FinishReason
	if result.StopReason == "stop" {
		result.StopReason = "end_turn"
	}
	result.Content = choice.Message.Content

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, model.ToolCallRequest{
			ID:        tc.ID,
			ToolName:  tc.Function.Name,
			InputJSON: []byte(tc.Function.Arguments),
		})
	}
	return result
}

func parseResponsesResponse(resp *openairesponses.Response) runtime.GenerateResult {
	result := runtime.GenerateResult{StopReason: "end_turn"}
	if resp == nil {
		return result
	}

	result.InputTokens = int(resp.Usage.InputTokens)
	result.OutputTokens = int(resp.Usage.OutputTokens)
	result.ModelID = resp.Model

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
