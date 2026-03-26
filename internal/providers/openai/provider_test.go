package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

type recordingStreamSink struct {
	deltas    []string
	completed bool
}

func (s *recordingStreamSink) OnDelta(_ context.Context, text string) error {
	s.deltas = append(s.deltas, text)
	return nil
}

func (s *recordingStreamSink) OnComplete() error {
	s.completed = true
	return nil
}

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

	p := New("test-key", "gpt-4o", srv.URL, "")
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
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason: got %q, want %q", result.StopReason, "end_turn")
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

	p := New("key", "gpt-4o-mini", srv.URL, "")
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

	p := New("key", "gpt-4o", srv.URL, "")
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

	p := New("key", "gpt-4o", srv.URL, "")
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

func TestProvider_ToolCallEventsConvertedToMessages(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("response", 5, 3))
	}))
	defer srv.Close()

	toolPayload, _ := json.Marshal(map[string]any{
		"tool_call_id": "call_123",
		"tool_name":    "read_file",
		"input_json":   map[string]any{"path": "main.go"},
		"output_json":  map[string]any{"output": "package main\n", "error": ""},
		"decision":     "allow",
	})

	p := New("key", "gpt-4o", srv.URL, "")
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
	if !ok {
		t.Fatalf("expected messages array, got %T", capturedBody["messages"])
	}

	var assistantToolMsg map[string]any
	var toolResultMsg map[string]any
	for _, raw := range msgs {
		msg := raw.(map[string]any)
		if msg["role"] == "assistant" {
			if toolCalls, ok := msg["tool_calls"].([]any); ok && len(toolCalls) == 1 {
				assistantToolMsg = msg
			}
		}
		if msg["role"] == "tool" {
			toolResultMsg = msg
		}
	}

	if assistantToolMsg == nil {
		t.Fatalf("expected assistant tool-call message, got %v", msgs)
	}
	if toolResultMsg == nil {
		t.Fatalf("expected tool result message, got %v", msgs)
	}
	if toolResultMsg["tool_call_id"] != "call_123" {
		t.Fatalf("unexpected tool_call_id %v", toolResultMsg["tool_call_id"])
	}
}

func TestProvider_TruncatesOversizedToolResultMessages(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("response", 5, 3))
	}))
	defer srv.Close()

	toolPayload, _ := json.Marshal(map[string]any{
		"tool_call_id": "call_oversized",
		"tool_name":    "web_fetch",
		"input_json":   map[string]any{"url": "https://example.com"},
		"output_json":  map[string]any{"output": strings.Repeat("a", 20000), "error": ""},
		"decision":     "allow",
	})

	p := New("key", "gpt-4o", srv.URL, "")
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
	if !ok {
		t.Fatalf("expected messages array, got %T", capturedBody["messages"])
	}

	for _, raw := range msgs {
		msg := raw.(map[string]any)
		if msg["role"] != "tool" {
			continue
		}
		content, ok := msg["content"].(string)
		if !ok {
			t.Fatalf("expected tool content string, got %T", msg["content"])
		}
		if !strings.Contains(content, "tool result truncated") {
			t.Fatalf("expected tool result content to include truncation marker, got %q", content)
		}
		if len(content) >= 20000 {
			t.Fatalf("expected truncated tool result content, got len=%d", len(content))
		}
		return
	}

	t.Fatalf("expected tool result message, got %v", msgs)
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

	p := New("key", "gpt-4o", srv.URL, "")
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

	p := New("bad-key", "gpt-4o", srv.URL, "")
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "system",
		MaxTokens:    100,
	}, nil)
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
}

func TestProvider_IDReturnsOpenAI(t *testing.T) {
	p := New("key", "gpt-4o", "", "")
	if p.ID() != "openai" {
		t.Errorf("ID: got %q, want %q", p.ID(), "openai")
	}
}

func TestProvider_BaseURLWithV1SuffixDoesNotDuplicateSegment(t *testing.T) {
	var requestPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("ok", 1, 1))
	}))
	defer srv.Close()

	p := New("test-key", "gpt-4o", srv.URL+"/v1", "chat_completions")
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "You are helpful.",
		MaxTokens:    128,
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if requestPath != "/v1/chat/completions" {
		t.Fatalf("request path: got %q, want %q", requestPath, "/v1/chat/completions")
	}
}

func TestProvider_ResponsesWireAPIReturnsText(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("request path: got %q, want %q", r.URL.Path, "/v1/responses")
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_test",
			"status":"completed",
			"output":[
				{
					"type":"message",
					"id":"msg_test",
					"status":"completed",
					"role":"assistant",
					"content":[
						{"type":"output_text","text":"Hello from Responses","annotations":[]}
					]
				}
			],
			"usage":{"input_tokens":11,"output_tokens":7}
		}`))
	}))
	defer srv.Close()

	p := New("test-key", "cx/gpt-5.4", srv.URL, "responses")
	result, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "You are helpful.",
		MaxTokens:    256,
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Content != "Hello from Responses" {
		t.Fatalf("Content: got %q, want %q", result.Content, "Hello from Responses")
	}
	if result.InputTokens != 11 || result.OutputTokens != 7 {
		t.Fatalf("usage: got %d/%d, want 11/7", result.InputTokens, result.OutputTokens)
	}
	if capturedBody["instructions"] != "You are helpful." {
		t.Fatalf("instructions: got %v, want %q", capturedBody["instructions"], "You are helpful.")
	}
	if capturedBody["max_output_tokens"] != float64(256) {
		t.Fatalf("max_output_tokens: got %v, want 256", capturedBody["max_output_tokens"])
	}
}

func TestProvider_ResponsesWireAPIParsesFunctionCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("request path: got %q, want %q", r.URL.Path, "/v1/responses")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_tool",
			"status":"completed",
			"output":[
				{
					"type":"function_call",
					"id":"fc_123",
					"call_id":"call_123",
					"name":"read_file",
					"arguments":"{\"path\":\"main.go\"}"
				}
			],
			"usage":{"input_tokens":20,"output_tokens":10}
		}`))
	}))
	defer srv.Close()

	p := New("test-key", "cx/gpt-5.4", srv.URL, "responses")
	result, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "Use tools when needed.",
		MaxTokens:    256,
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ID != "call_123" {
		t.Fatalf("tool call ID: got %q, want %q", result.ToolCalls[0].ID, "call_123")
	}
	if result.ToolCalls[0].ToolName != "read_file" {
		t.Fatalf("tool call name: got %q, want %q", result.ToolCalls[0].ToolName, "read_file")
	}
	if string(result.ToolCalls[0].InputJSON) != `{"path":"main.go"}` {
		t.Fatalf("tool call arguments: got %s", result.ToolCalls[0].InputJSON)
	}
}

func TestProvider_ResponsesWireAPIToolCallEventsConvertedToInput(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("request path: got %q, want %q", r.URL.Path, "/v1/responses")
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_test",
			"status":"completed",
			"output":[
				{
					"type":"message",
					"id":"msg_test",
					"status":"completed",
					"role":"assistant",
					"content":[
						{"type":"output_text","text":"done","annotations":[]}
					]
				}
			],
			"usage":{"input_tokens":11,"output_tokens":7}
		}`))
	}))
	defer srv.Close()

	toolPayload, _ := json.Marshal(map[string]any{
		"tool_call_id": "call_123",
		"tool_name":    "read_file",
		"input_json":   map[string]any{"path": "main.go"},
		"output_json":  map[string]any{"output": "package main\n", "error": ""},
		"decision":     "allow",
	})

	p := New("test-key", "cx/gpt-5.4", srv.URL, "responses")
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "Use tools when needed.",
		MaxTokens:    256,
		ConversationCtx: []model.Event{
			{Kind: "tool_call_recorded", PayloadJSON: toolPayload},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	input, ok := capturedBody["input"].([]any)
	if !ok || len(input) < 2 {
		t.Fatalf("expected function_call and function_call_output input items, got %v", capturedBody["input"])
	}
	if input[0].(map[string]any)["type"] != "function_call" {
		t.Fatalf("expected first input item to be function_call, got %v", input[0])
	}
	if input[1].(map[string]any)["type"] != "function_call_output" {
		t.Fatalf("expected second input item to be function_call_output, got %v", input[1])
	}
}

func TestProvider_InvalidToolSchemaFallsBackToEmptyObject(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(successResponse("ok", 1, 1))
	}))
	defer srv.Close()

	p := New("test-key", "gpt-4o", srv.URL, "chat_completions")
	_, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "Use tools when needed.",
		MaxTokens:    64,
		ToolSpecs: []model.ToolSpec{
			{
				Name:            "read_file",
				Description:     "Read a file.",
				InputSchemaJSON: `{"type":"object",`,
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	tools, ok := capturedBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected one tool definition, got %v", capturedBody["tools"])
	}
	function, ok := tools[0].(map[string]any)["function"].(map[string]any)
	if !ok {
		t.Fatalf("expected function tool payload, got %v", tools[0])
	}
	parameters, ok := function["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected parameters object fallback, got %T", function["parameters"])
	}
	if parameters["type"] != "object" {
		t.Fatalf("parameters.type: got %v, want object", parameters["type"])
	}
}

func TestProvider_StreamingChatCompletionsWritesTextDeltas(t *testing.T) {
	sseBody := `data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":0,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"},"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":0,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":"stop"}],"usage":null}

data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":0,"model":"gpt-4o","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	sink := &recordingStreamSink{}
	p := New("test-key", "gpt-4o", srv.URL, "chat_completions")
	result, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "You are helpful.",
		MaxTokens:    128,
	}, sink)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if got := len(sink.deltas); got != 2 {
		t.Fatalf("expected 2 streamed deltas, got %d", got)
	}
	if sink.deltas[0] != "Hel" || sink.deltas[1] != "lo" {
		t.Fatalf("unexpected streamed deltas: %v", sink.deltas)
	}
	if !sink.completed {
		t.Fatal("expected stream completion callback")
	}
	if result.Content != "Hello" {
		t.Fatalf("Content: got %q, want %q", result.Content, "Hello")
	}
	if result.InputTokens != 12 || result.OutputTokens != 3 {
		t.Fatalf("usage: got %d/%d, want 12/3", result.InputTokens, result.OutputTokens)
	}
}

func TestProvider_StreamingResponsesWritesTextDeltas(t *testing.T) {
	sseBody := `data: {"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Hel","logprobs":[]}

data: {"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"lo","logprobs":[]}

data: {"type":"response.completed","sequence_number":3,"response":{"id":"resp_1","status":"completed","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello","annotations":[]}]}],"usage":{"input_tokens":11,"output_tokens":7}}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	sink := &recordingStreamSink{}
	p := New("test-key", "cx/gpt-5.4", srv.URL, "responses")
	result, err := p.Generate(context.Background(), runtime.GenerateRequest{
		Instructions: "You are helpful.",
		MaxTokens:    128,
	}, sink)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if got := len(sink.deltas); got != 2 {
		t.Fatalf("expected 2 streamed deltas, got %d", got)
	}
	if sink.deltas[0] != "Hel" || sink.deltas[1] != "lo" {
		t.Fatalf("unexpected streamed deltas: %v", sink.deltas)
	}
	if !sink.completed {
		t.Fatal("expected stream completion callback")
	}
	if result.Content != "Hello" {
		t.Fatalf("Content: got %q, want %q", result.Content, "Hello")
	}
	if result.InputTokens != 11 || result.OutputTokens != 7 {
		t.Fatalf("usage: got %d/%d, want 11/7", result.InputTokens, result.OutputTokens)
	}
}
