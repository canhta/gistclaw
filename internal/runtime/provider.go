package runtime

import (
	"context"

	"github.com/canhta/gistclaw/internal/model"
)

type StreamSink interface {
	OnDelta(ctx context.Context, text string) error
	OnComplete() error
}

type GenerateRequest struct {
	Instructions    string
	ConversationCtx []model.Event
	ToolSpecs       []model.ToolSpec
	ModelID         string
	MaxTokens       int
	AttachmentRefs  []string
}

type GenerateResult struct {
	Content      string
	ToolCalls    []model.ToolCallRequest
	InputTokens  int
	OutputTokens int
	StopReason   string
	ModelID      string
}

type Provider interface {
	ID() string
	Generate(ctx context.Context, req GenerateRequest, stream StreamSink) (GenerateResult, error)
}

type MockProvider struct {
	Responses []GenerateResult
	Errors    []error
	callCount int
	Requests  []GenerateRequest
}

func NewMockProvider(responses []GenerateResult, errors []error) *MockProvider {
	return &MockProvider{
		Responses: responses,
		Errors:    errors,
	}
}

func (m *MockProvider) ID() string {
	return "mock"
}

func (m *MockProvider) Generate(_ context.Context, req GenerateRequest, _ StreamSink) (GenerateResult, error) {
	index := m.callCount
	m.callCount++
	m.Requests = append(m.Requests, req)

	if index < len(m.Errors) && m.Errors[index] != nil {
		return GenerateResult{}, m.Errors[index]
	}
	if index < len(m.Responses) {
		return m.Responses[index], nil
	}

	return GenerateResult{
		Content:      "mock response",
		InputTokens:  10,
		OutputTokens: 20,
	}, nil
}

func (m *MockProvider) CallCount() int {
	return m.callCount
}
