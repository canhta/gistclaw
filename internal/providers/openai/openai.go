// internal/providers/openai/openai.go
package openai

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/canhta/gistclaw/internal/providers"
)

// pricingTable maps model name to (inputPerMToken, outputPerMToken) in USD.
var pricingTable = map[string][2]float64{
	"gpt-4o":      {2.50, 10.00},
	"gpt-4o-mini": {0.15, 0.60},
}

// Provider implements providers.LLMProvider using the OpenAI API key path.
type Provider struct {
	client openai.Client
	model  string
}

// New creates an OpenAI provider using the official SDK with the default base URL.
func New(apiKey, model string) *Provider {
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &Provider{client: client, model: model}
}

// NewWithBaseURL creates an OpenAI provider pointing at a custom base URL.
// Used in tests to point at httptest.Server.
func NewWithBaseURL(apiKey, model, baseURL string) *Provider {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	)
	return &Provider{client: client, model: model}
}

// Name implements providers.LLMProvider.
func (p *Provider) Name() string { return "openai" }

// Chat implements providers.LLMProvider.
func (p *Provider) Chat(ctx context.Context, messages []providers.Message, tools []providers.Tool) (*providers.LLMResponse, error) {
	params := openai.ChatCompletionNewParams{
		Model:    p.model,
		Messages: convertMessages(messages),
	}
	if len(tools) > 0 {
		params.Tools = convertTools(tools)
	}

	completion, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai: chat completions: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	choice := completion.Choices[0]
	resp := &providers.LLMResponse{
		Content: choice.Message.Content,
		Usage: providers.Usage{
			PromptTokens:     int(completion.Usage.PromptTokens),
			CompletionTokens: int(completion.Usage.CompletionTokens),
			TotalCostUSD:     computeCost(p.model, int(completion.Usage.PromptTokens), int(completion.Usage.CompletionTokens)),
		},
	}

	if len(choice.Message.ToolCalls) > 0 {
		tc := choice.Message.ToolCalls[0]
		resp.ToolCall = &providers.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			InputJSON: tc.Function.Arguments,
		}
	}

	return resp, nil
}

// computeCost returns the total cost in USD for the given token counts and model.
// Returns 0 for unknown models.
func computeCost(model string, promptTokens, completionTokens int) float64 {
	pricing, ok := pricingTable[model]
	if !ok {
		return 0
	}
	inputCost := float64(promptTokens) * pricing[0] / 1e6
	outputCost := float64(completionTokens) * pricing[1] / 1e6
	return inputCost + outputCost
}

// convertMessages converts providers.Message slice to the openai SDK format.
func convertMessages(messages []providers.Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "user":
			out = append(out, openai.UserMessage(m.Content))
		case "assistant":
			if m.ToolName != "" {
				// This assistant message represents a tool call request.
				// OpenAI requires a tool_calls array; plain content is not accepted
				// as a precursor to a role=tool result message.
				out = append(out, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{
							{
								OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
									ID: m.ToolCallID,
									Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
										Name:      m.ToolName,
										Arguments: m.Content,
									},
								},
							},
						},
					},
				})
			} else {
				out = append(out, openai.AssistantMessage(m.Content))
			}
		case "system":
			out = append(out, openai.SystemMessage(m.Content))
		case "tool":
			out = append(out, openai.ToolMessage(m.Content, m.ToolCallID))
		}
	}
	return out
}

// convertTools converts providers.Tool slice to the openai SDK tool format.
func convertTools(tools []providers.Tool) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  shared.FunctionParameters(t.InputSchema),
		}))
	}
	return out
}
