// internal/providers/openai/openai_test.go
package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/providers"
	oaiprovider "github.com/canhta/gistclaw/internal/providers/openai"
)

// chatResponse is the minimal OpenAI chat completions response shape we produce
// in the mock. The real SDK parses the full OpenAPI schema, so we must match it.
type chatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

func mockOpenAIServer(t *testing.T, response chatResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("mock server encode: %v", err)
		}
	}))
}

func buildTextResponse(content string, promptTokens, completionTokens int, model string) chatResponse {
	resp := chatResponse{
		ID:     "chatcmpl-test",
		Object: "chat.completion",
		Model:  model,
	}
	resp.Choices = []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}{
		{
			Index:        0,
			FinishReason: "stop",
			Message: struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls,omitempty"`
			}{
				Role:    "assistant",
				Content: content,
			},
		},
	}
	resp.Usage.PromptTokens = promptTokens
	resp.Usage.CompletionTokens = completionTokens
	resp.Usage.TotalTokens = promptTokens + completionTokens
	return resp
}

func TestOpenAIChatTextResponse(t *testing.T) {
	mock := mockOpenAIServer(t, buildTextResponse("Hello, world!", 100, 50, "gpt-4o"))
	defer mock.Close()

	p := oaiprovider.NewWithBaseURL("test-api-key", "gpt-4o", mock.URL+"/v1")

	resp, err := p.Chat(context.Background(), []providers.Message{
		{Role: "user", Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.Content != "Hello, world!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello, world!")
	}
	if resp.ToolCall != nil {
		t.Errorf("ToolCall should be nil for text response")
	}
	if resp.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50", resp.Usage.CompletionTokens)
	}
	// gpt-4o: $2.50/1M input + $10.00/1M output
	// 100 * 2.50/1e6 + 50 * 10.00/1e6 = 0.000250 + 0.000500 = 0.000750
	wantCost := 100.0*2.50/1e6 + 50.0*10.00/1e6
	if resp.Usage.TotalCostUSD != wantCost {
		t.Errorf("TotalCostUSD = %v, want %v", resp.Usage.TotalCostUSD, wantCost)
	}
}

func TestOpenAIChatToolCallResponse(t *testing.T) {
	toolResp := buildTextResponse("", 200, 30, "gpt-4o")
	// Replace content with a tool call.
	toolResp.Choices[0].Message.Content = ""
	toolResp.Choices[0].FinishReason = "tool_calls"
	toolResp.Choices[0].Message.ToolCalls = []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}{
		{
			ID:   "call_abc123",
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      "web_search",
				Arguments: `{"query":"latest Go release"}`,
			},
		},
	}

	mock := mockOpenAIServer(t, toolResp)
	defer mock.Close()

	p := oaiprovider.NewWithBaseURL("test-api-key", "gpt-4o", mock.URL+"/v1")

	resp, err := p.Chat(context.Background(), []providers.Message{
		{Role: "user", Content: "what's the latest Go?"},
	}, []providers.Tool{
		{Name: "web_search", Description: "search", InputSchema: map[string]any{"type": "object"}},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.ToolCall == nil {
		t.Fatal("expected ToolCall, got nil")
	}
	if resp.ToolCall.Name != "web_search" {
		t.Errorf("ToolCall.Name = %q, want %q", resp.ToolCall.Name, "web_search")
	}
	if resp.ToolCall.ID != "call_abc123" {
		t.Errorf("ToolCall.ID = %q, want %q", resp.ToolCall.ID, "call_abc123")
	}
	if resp.ToolCall.InputJSON != `{"query":"latest Go release"}` {
		t.Errorf("ToolCall.InputJSON = %q", resp.ToolCall.InputJSON)
	}
}

func TestOpenAIChatGPT4oMiniCost(t *testing.T) {
	mock := mockOpenAIServer(t, buildTextResponse("ok", 1000, 500, "gpt-4o-mini"))
	defer mock.Close()

	p := oaiprovider.NewWithBaseURL("test-api-key", "gpt-4o-mini", mock.URL+"/v1")

	resp, err := p.Chat(context.Background(), []providers.Message{
		{Role: "user", Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	// gpt-4o-mini: $0.15/1M input + $0.60/1M output
	// 1000 * 0.15/1e6 + 500 * 0.60/1e6 = 0.000150 + 0.000300 = 0.000450
	wantCost := 1000.0*0.15/1e6 + 500.0*0.60/1e6
	if resp.Usage.TotalCostUSD != wantCost {
		t.Errorf("TotalCostUSD = %v, want %v", resp.Usage.TotalCostUSD, wantCost)
	}
}

func TestOpenAIChatUnknownModelCostIsZero(t *testing.T) {
	mock := mockOpenAIServer(t, buildTextResponse("hi", 100, 100, "gpt-5-unknown"))
	defer mock.Close()

	p := oaiprovider.NewWithBaseURL("test-api-key", "gpt-5-unknown", mock.URL+"/v1")

	resp, err := p.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.Usage.TotalCostUSD != 0 {
		t.Errorf("unknown model TotalCostUSD = %v, want 0", resp.Usage.TotalCostUSD)
	}
}

// TestOpenAIConvertMessagesToolCallAssistant verifies that an assistant message
// with ToolName set is serialised as an OpenAI tool_calls array (not plain text).
// Without the fix, the request body contains `"role":"assistant","content":"..."` with
// no tool_calls array, causing a 400 from the OpenAI API.
func TestOpenAIConvertMessagesToolCallAssistant(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		// Return a minimal valid text response so the SDK doesn't error.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildTextResponse("ok", 10, 5, "gpt-4o")) //nolint:errcheck
	}))
	defer srv.Close()

	p := oaiprovider.NewWithBaseURL("test-api-key", "gpt-4o", srv.URL+"/v1")

	msgs := []providers.Message{
		{Role: "user", Content: "search for Go"},
		// This is the assistant message that loop.go appends after a tool call.
		{Role: "assistant", Content: `{"query":"Go"}`, ToolCallID: "call_xyz", ToolName: "web_search"},
		{Role: "tool", Content: "Go 1.25 released", ToolCallID: "call_xyz"},
	}
	_, err := p.Chat(context.Background(), msgs, nil)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}

	// Parse the request body to verify the assistant message has tool_calls.
	var reqBody struct {
		Messages []struct {
			Role      string `json:"role"`
			Content   string `json:"content,omitempty"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
			ToolCallID string `json:"tool_call_id,omitempty"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(capturedBody, &reqBody); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}

	// Find the assistant message (index 1).
	if len(reqBody.Messages) < 3 {
		t.Fatalf("expected 3 messages, got %d", len(reqBody.Messages))
	}
	asst := reqBody.Messages[1]
	if asst.Role != "assistant" {
		t.Errorf("messages[1].role = %q, want %q", asst.Role, "assistant")
	}
	if len(asst.ToolCalls) != 1 {
		t.Fatalf("messages[1].tool_calls len = %d, want 1 (got: %+v)", len(asst.ToolCalls), asst)
	}
	tc := asst.ToolCalls[0]
	if tc.ID != "call_xyz" {
		t.Errorf("tool_calls[0].id = %q, want %q", tc.ID, "call_xyz")
	}
	if tc.Type != "function" {
		t.Errorf("tool_calls[0].type = %q, want %q", tc.Type, "function")
	}
	if tc.Function.Name != "web_search" {
		t.Errorf("tool_calls[0].function.name = %q, want %q", tc.Function.Name, "web_search")
	}
	if tc.Function.Arguments != `{"query":"Go"}` {
		t.Errorf("tool_calls[0].function.arguments = %q, want %q", tc.Function.Arguments, `{"query":"Go"}`)
	}
}

func TestOpenAIName(t *testing.T) {
	p := oaiprovider.NewWithBaseURL("key", "gpt-4o", "http://localhost")
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai")
	}
}

func TestOpenAIServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"internal error"}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	p := oaiprovider.NewWithBaseURL("key", "gpt-4o", srv.URL+"/v1")
	_, err := p.Chat(context.Background(), []providers.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error on 5xx response")
	}
}
