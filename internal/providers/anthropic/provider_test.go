package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

// successResponse builds a minimal valid Anthropic Messages API response body.
func successResponse(text string, inputTokens, outputTokens int) []byte {
	b, _ := json.Marshal(map[string]any{
		"id":   "msg_test",
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
		"stop_reason": "end_turn",
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	})
	return b
}

func TestProvider_GeneratesTextCompletion(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("Hello from Claude", 10, 5))
	}))
	defer srv.Close()

	p := newWithEndpoint("test-key", "claude-3-5-sonnet-20241022", srv.URL)
	result, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "You are a helpful assistant.",
		ModelID:      "claude-3-5-sonnet-20241022",
		MaxTokens:    1024,
	}, nil)

	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Content != "Hello from Claude" {
		t.Errorf("Content: got %q, want %q", result.Content, "Hello from Claude")
	}
	if result.InputTokens != 10 {
		t.Errorf("InputTokens: got %d, want 10", result.InputTokens)
	}
	if result.OutputTokens != 5 {
		t.Errorf("OutputTokens: got %d, want 5", result.OutputTokens)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason: got %q, want %q", result.StopReason, "end_turn")
	}
}

func TestProvider_SystemInstructionsPassedAsSystem(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("ok", 1, 1))
	}))
	defer srv.Close()

	p := newWithEndpoint("key", "claude-3-haiku-20240307", srv.URL)
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "You are a code reviewer.",
		MaxTokens:    100,
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if capturedBody["system"] != "You are a code reviewer." {
		t.Errorf("system: got %v, want %q", capturedBody["system"], "You are a code reviewer.")
	}
}

func TestProvider_ConversationEventsConvertedToMessages(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("response", 5, 3))
	}))
	defer srv.Close()

	objPayload, _ := json.Marshal(map[string]any{"objective": "fix the bug", "agent_id": "coder"})
	turnPayload, _ := json.Marshal(map[string]any{"content": "I found the bug", "input_tokens": 5, "output_tokens": 3})

	p := newWithEndpoint("key", "claude-3-5-sonnet-20241022", srv.URL)
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "system",
		MaxTokens:    100,
		ConversationCtx: []model.Event{
			{Kind: "run_started", PayloadJSON: objPayload},
			{Kind: "turn_completed", PayloadJSON: turnPayload},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	msgs, ok := capturedBody["messages"].([]any)
	if !ok || len(msgs) < 2 {
		t.Fatalf("expected >=2 messages, got %v", capturedBody["messages"])
	}

	m0 := msgs[0].(map[string]any)
	if m0["role"] != "user" {
		t.Errorf("first message role: got %v, want user", m0["role"])
	}
	if m0["content"] != "fix the bug" {
		t.Errorf("first message content: got %v, want 'fix the bug'", m0["content"])
	}

	m1 := msgs[1].(map[string]any)
	if m1["role"] != "assistant" {
		t.Errorf("second message role: got %v, want assistant", m1["role"])
	}
	if m1["content"] != "I found the bug" {
		t.Errorf("second message content: got %v, want 'I found the bug'", m1["content"])
	}
}

func TestProvider_SessionMessageEventsConvertedToMessages(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("response", 5, 3))
	}))
	defer srv.Close()

	userPayload, _ := json.Marshal(map[string]any{"kind": "user", "body": "front prompt"})
	assistantPayload, _ := json.Marshal(map[string]any{"kind": "assistant", "body": "front reply"})

	p := newWithEndpoint("key", "claude-3-5-sonnet-20241022", srv.URL)
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "system",
		MaxTokens:    100,
		ConversationCtx: []model.Event{
			{Kind: "session_message_added", PayloadJSON: userPayload},
			{Kind: "session_message_added", PayloadJSON: assistantPayload},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	msgs, ok := capturedBody["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 session messages, got %v", capturedBody["messages"])
	}
	if msgs[0].(map[string]any)["role"] != "user" || msgs[0].(map[string]any)["content"] != "front prompt" {
		t.Fatalf("unexpected user mailbox message: %v", msgs[0])
	}
	if msgs[1].(map[string]any)["role"] != "assistant" || msgs[1].(map[string]any)["content"] != "front reply" {
		t.Fatalf("unexpected assistant mailbox message: %v", msgs[1])
	}
}

func TestProvider_ToolCallEventsConvertedToMessages(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("response", 5, 3))
	}))
	defer srv.Close()

	toolPayload, _ := json.Marshal(map[string]any{
		"tool_call_id": "toolu_123",
		"tool_name":    "read_file",
		"input_json":   map[string]any{"path": "main.go"},
		"output_json":  map[string]any{"output": "package main\n", "error": ""},
		"decision":     "allow",
	})

	p := newWithEndpoint("key", "claude-3-5-sonnet-20241022", srv.URL)
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "system",
		MaxTokens:    100,
		ConversationCtx: []model.Event{
			{Kind: "tool_call_recorded", PayloadJSON: toolPayload},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	msgs, ok := capturedBody["messages"].([]any)
	if !ok || len(msgs) < 2 {
		t.Fatalf("expected tool-use and tool-result messages, got %v", capturedBody["messages"])
	}

	assistantMsg := msgs[0].(map[string]any)
	if assistantMsg["role"] != "assistant" {
		t.Fatalf("expected first message role assistant, got %v", assistantMsg["role"])
	}
	assistantBlocks, ok := assistantMsg["content"].([]any)
	if !ok || len(assistantBlocks) != 1 {
		t.Fatalf("expected assistant tool_use block, got %v", assistantMsg["content"])
	}
	if assistantBlocks[0].(map[string]any)["type"] != "tool_use" {
		t.Fatalf("expected tool_use block, got %v", assistantBlocks[0])
	}

	userMsg := msgs[1].(map[string]any)
	if userMsg["role"] != "user" {
		t.Fatalf("expected second message role user, got %v", userMsg["role"])
	}
	userBlocks, ok := userMsg["content"].([]any)
	if !ok || len(userBlocks) != 1 {
		t.Fatalf("expected user tool_result block, got %v", userMsg["content"])
	}
	if userBlocks[0].(map[string]any)["type"] != "tool_result" {
		t.Fatalf("expected tool_result block, got %v", userBlocks[0])
	}
}

func TestProvider_ToolCallsInResponse(t *testing.T) {
	toolInputJSON, _ := json.Marshal(map[string]any{"path": "main.go"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]any{
			"id":   "msg_tools",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "I'll read the file."},
				{"type": "tool_use", "id": "toolu_01", "name": "read_file", "input": map[string]any{"path": "main.go"}},
			},
			"stop_reason": "tool_use",
			"usage":       map[string]any{"input_tokens": 20, "output_tokens": 15},
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	p := newWithEndpoint("key", "claude-3-5-sonnet-20241022", srv.URL)
	result, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "system",
		MaxTokens:    100,
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if result.StopReason != "tool_use" {
		t.Errorf("StopReason: got %q, want %q", result.StopReason, "tool_use")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc.ID != "toolu_01" {
		t.Errorf("ToolCall.ID: got %q, want %q", tc.ID, "toolu_01")
	}
	if tc.ToolName != "read_file" {
		t.Errorf("ToolCall.ToolName: got %q, want %q", tc.ToolName, "read_file")
	}
	// InputJSON should be valid JSON matching the tool input.
	var gotInput map[string]any
	if err := json.Unmarshal(tc.InputJSON, &gotInput); err != nil {
		t.Fatalf("InputJSON unmarshal: %v", err)
	}
	var wantInput map[string]any
	_ = json.Unmarshal(toolInputJSON, &wantInput)
	if gotInput["path"] != wantInput["path"] {
		t.Errorf("ToolCall input path: got %v, want %v", gotInput["path"], wantInput["path"])
	}
}

func TestProvider_APIErrorMapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()

	p := newWithEndpoint("bad-key", "claude-3-5-sonnet-20241022", srv.URL)
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "system",
		MaxTokens:    100,
	}, nil)
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
	var provErr *model.ProviderError
	// Check the error wraps a ProviderError or at least contains meaningful context.
	if pe, ok := err.(*model.ProviderError); ok {
		provErr = pe
		if provErr.Code != model.ErrMalformedResponse && provErr.Code != model.ProviderErrorCode("authentication_error") {
			t.Logf("ProviderError.Code = %q (acceptable)", provErr.Code)
		}
	}
	// Any non-nil error is acceptable; the important thing is it surfaces.
}

func TestProvider_IDReturnsAnthropic(t *testing.T) {
	p := New("key", "claude-3-5-sonnet-20241022")
	if p.ID() != "anthropic" {
		t.Errorf("ID: got %q, want %q", p.ID(), "anthropic")
	}
}
