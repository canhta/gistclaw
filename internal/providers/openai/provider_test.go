package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

func successResponse(text string, promptTokens, completionTokens int) []byte {
	b, _ := json.Marshal(map[string]any{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": text,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
		},
	})
	return b
}

func TestProvider_GeneratesTextCompletion(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("Hello from GPT", 10, 5))
	}))
	defer srv.Close()

	p := New("test-key", "gpt-4o", srv.URL)
	result, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "You are helpful.",
		ModelID:      "gpt-4o",
		MaxTokens:    1024,
	}, nil)

	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Content != "Hello from GPT" {
		t.Errorf("Content: got %q, want %q", result.Content, "Hello from GPT")
	}
	if result.InputTokens != 10 {
		t.Errorf("InputTokens: got %d, want 10", result.InputTokens)
	}
	if result.OutputTokens != 5 {
		t.Errorf("OutputTokens: got %d, want 5", result.OutputTokens)
	}
	if result.StopReason != "stop" {
		t.Errorf("StopReason: got %q, want %q", result.StopReason, "stop")
	}
}

func TestProvider_SystemInstructionsPassedAsSystemMessage(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("ok", 1, 1))
	}))
	defer srv.Close()

	p := New("key", "gpt-4o-mini", srv.URL)
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "You are a code reviewer.",
		MaxTokens:    100,
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	msgs, ok := capturedBody["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected messages, got %v", capturedBody["messages"])
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Errorf("first message role: got %v, want system", first["role"])
	}
	if first["content"] != "You are a code reviewer." {
		t.Errorf("system content: got %v, want %q", first["content"], "You are a code reviewer.")
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
	turnPayload, _ := json.Marshal(map[string]any{"content": "I found it", "input_tokens": 5, "output_tokens": 3})

	p := New("key", "gpt-4o", srv.URL)
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

	msgs, _ := capturedBody["messages"].([]any)
	// msgs[0] is system; subsequent are conversation events.
	var userMsg, assistantMsg map[string]any
	for _, m := range msgs {
		msg := m.(map[string]any)
		switch msg["role"] {
		case "user":
			userMsg = msg
		case "assistant":
			assistantMsg = msg
		}
	}

	if userMsg == nil {
		t.Fatal("expected a user message")
	}
	if userMsg["content"] != "fix the bug" {
		t.Errorf("user content: got %v, want 'fix the bug'", userMsg["content"])
	}
	if assistantMsg == nil {
		t.Fatal("expected an assistant message")
	}
	if assistantMsg["content"] != "I found it" {
		t.Errorf("assistant content: got %v, want 'I found it'", assistantMsg["content"])
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

	p := New("key", "gpt-4o", srv.URL)
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

	msgs, _ := capturedBody["messages"].([]any)
	if len(msgs) < 3 {
		t.Fatalf("expected system + 2 session messages, got %v", capturedBody["messages"])
	}
	if msgs[1].(map[string]any)["role"] != "user" || msgs[1].(map[string]any)["content"] != "front prompt" {
		t.Fatalf("unexpected user mailbox message: %v", msgs[1])
	}
	if msgs[2].(map[string]any)["role"] != "assistant" || msgs[2].(map[string]any)["content"] != "front reply" {
		t.Fatalf("unexpected assistant mailbox message: %v", msgs[2])
	}
}

func TestProvider_ToolCallsInResponse(t *testing.T) {
	argsJSON, _ := json.Marshal(map[string]any{"path": "main.go"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]any{
			"id":     "chatcmpl-tools",
			"object": "chat.completion",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]any{
							{
								"id":   "call_01",
								"type": "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": string(argsJSON),
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 10},
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	p := New("key", "gpt-4o", srv.URL)
	result, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "system",
		MaxTokens:    100,
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if result.StopReason != "tool_calls" {
		t.Errorf("StopReason: got %q, want %q", result.StopReason, "tool_calls")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc.ID != "call_01" {
		t.Errorf("ToolCall.ID: got %q, want %q", tc.ID, "call_01")
	}
	if tc.ToolName != "read_file" {
		t.Errorf("ToolCall.ToolName: got %q, want %q", tc.ToolName, "read_file")
	}
}

func TestProvider_APIErrorMapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"invalid API key"}}`))
	}))
	defer srv.Close()

	p := New("bad-key", "gpt-4o", srv.URL)
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "system",
		MaxTokens:    100,
	}, nil)
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
}

func TestProvider_IDReturnsOpenAI(t *testing.T) {
	p := New("key", "gpt-4o", "")
	if p.ID() != "openai" {
		t.Errorf("ID: got %q, want %q", p.ID(), "openai")
	}
}
