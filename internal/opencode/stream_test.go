// internal/opencode/stream_test.go
package opencode_test

import (
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/opencode"
)

func TestParseSSELine_TextPart(t *testing.T) {
	line := `data: {"type":"message.part.updated","part":{"type":"text","text":"hello"}}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.Type != "message.part.updated" {
		t.Errorf("Type: got %q, want message.part.updated", ev.Type)
	}
	if ev.Part == nil {
		t.Fatal("expected Part, got nil")
	}
	if ev.Part.Type != "text" {
		t.Errorf("Part.Type: got %q, want text", ev.Part.Type)
	}
	if ev.Part.Text != "hello" {
		t.Errorf("Part.Text: got %q, want hello", ev.Part.Text)
	}
}

func TestParseSSELine_StepFinish(t *testing.T) {
	line := `data: {"type":"message.part.updated","part":{"type":"step-finish","cost_usd":0.0042}}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Part == nil {
		t.Fatal("expected Part, got nil")
	}
	if ev.Part.Type != "step-finish" {
		t.Errorf("Part.Type: got %q, want step-finish", ev.Part.Type)
	}
	if ev.Part.CostUSD != 0.0042 {
		t.Errorf("Part.CostUSD: got %v, want 0.0042", ev.Part.CostUSD)
	}
}

func TestParseSSELine_PermissionAsked(t *testing.T) {
	line := `data: {"type":"permission.asked","id":"perm_01","session_id":"sess_01","permission":"edit","patterns":["/tmp/foo.go"]}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "permission.asked" {
		t.Errorf("Type: got %q, want permission.asked", ev.Type)
	}
	if ev.ID != "perm_01" {
		t.Errorf("ID: got %q, want perm_01", ev.ID)
	}
	if ev.Permission != "edit" {
		t.Errorf("Permission: got %q, want edit", ev.Permission)
	}
	if len(ev.Patterns) != 1 || ev.Patterns[0] != "/tmp/foo.go" {
		t.Errorf("Patterns: got %v, want [\"/tmp/foo.go\"]", ev.Patterns)
	}
}

func TestParseSSELine_QuestionAsked(t *testing.T) {
	line := `data: {"type":"question.asked","id":"q_01","session_id":"sess_01","questions":[{"question":"Which framework?","header":"Test","options":[{"label":"testify","description":"Popular"}],"multiple":false,"custom":true}]}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "question.asked" {
		t.Errorf("Type: got %q, want question.asked", ev.Type)
	}
	if len(ev.Questions) != 1 {
		t.Fatalf("Questions: got %d, want 1", len(ev.Questions))
	}
	q := ev.Questions[0]
	if !q.Custom {
		t.Error("Custom must be true")
	}
}

func TestParseSSELine_SessionIdle(t *testing.T) {
	line := `data: {"type":"session.status","status":{"type":"idle"}}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "session.status" {
		t.Errorf("Type: got %q, want session.status", ev.Type)
	}
	if ev.Status == nil {
		t.Fatal("expected Status, got nil")
	}
	if ev.Status.Type != "idle" {
		t.Errorf("Status.Type: got %q, want idle", ev.Status.Type)
	}
}

func TestParseSSELine_NonDataLine(t *testing.T) {
	for _, line := range []string{"", ": keepalive", "event: ping"} {
		ev, err := opencode.ParseSSELine(line)
		if err != nil {
			t.Errorf("line %q: unexpected error: %v", line, err)
		}
		if ev != nil {
			t.Errorf("line %q: expected nil event, got %+v", line, ev)
		}
	}
}

func TestParseSSELine_MalformedJSON(t *testing.T) {
	line := "data: {not valid json"
	_, err := opencode.ParseSSELine(line)
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "SSE JSON") {
		t.Errorf("error should mention SSE JSON: %v", err)
	}
}
