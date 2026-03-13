package claudecode_test

import (
	"testing"

	"github.com/canhta/gistclaw/internal/claudecode"
)

func TestParseStreamLine_Text(t *testing.T) {
	line := `{"type":"text","text":"Hello from claude"}`
	ev, err := claudecode.ParseStreamLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.Type != "text" {
		t.Errorf("Type: got %q, want text", ev.Type)
	}
	if ev.Text != "Hello from claude" {
		t.Errorf("Text: got %q, want 'Hello from claude'", ev.Text)
	}
}

func TestParseStreamLine_ResultSuccess(t *testing.T) {
	line := `{"type":"result","total_cost_usd":0.0123,"is_error":false,"result":"done"}`
	ev, err := claudecode.ParseStreamLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "result" {
		t.Errorf("Type: got %q, want result", ev.Type)
	}
	if ev.TotalCostUSD != 0.0123 {
		t.Errorf("TotalCostUSD: got %v, want 0.0123", ev.TotalCostUSD)
	}
	if ev.IsError {
		t.Error("IsError must be false")
	}
	if ev.Result != "done" {
		t.Errorf("Result: got %q, want done", ev.Result)
	}
}

func TestParseStreamLine_ResultError(t *testing.T) {
	line := `{"type":"result","total_cost_usd":0,"is_error":true,"result":"something went wrong"}`
	ev, err := claudecode.ParseStreamLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ev.IsError {
		t.Error("IsError must be true")
	}
}

func TestParseStreamLine_EmptyLine(t *testing.T) {
	ev, err := claudecode.ParseStreamLine("")
	if err != nil {
		t.Errorf("empty line: unexpected error: %v", err)
	}
	if ev != nil {
		t.Errorf("empty line: expected nil event, got %+v", ev)
	}
}

func TestParseStreamLine_MalformedJSON(t *testing.T) {
	_, err := claudecode.ParseStreamLine("{not json")
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

func TestParseStreamLine_UnknownType(t *testing.T) {
	// Unknown types must not error — just return the parsed event.
	ev, err := claudecode.ParseStreamLine(`{"type":"assistant","message":{}}`)
	if err != nil {
		t.Fatalf("unexpected error for unknown type: %v", err)
	}
	if ev == nil {
		t.Fatal("expected non-nil event for unknown type")
	}
	if ev.Type != "assistant" {
		t.Errorf("Type: got %q, want assistant", ev.Type)
	}
}
