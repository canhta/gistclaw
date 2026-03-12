package hitl_test

import (
	"testing"

	"github.com/canhta/gistclaw/internal/hitl"
)

func TestHITLDecisionZeroValue(t *testing.T) {
	var d hitl.HITLDecision
	if d.Allow {
		t.Error("zero-value HITLDecision.Allow must be false (deny by default)")
	}
	if d.Always {
		t.Error("zero-value HITLDecision.Always must be false")
	}
	if d.Stop {
		t.Error("zero-value HITLDecision.Stop must be false")
	}
}

func TestPermissionRequestFields(t *testing.T) {
	ch := make(chan hitl.HITLDecision, 1)
	req := hitl.PermissionRequest{
		ChatID:     123,
		ID:         "permission_01ARZ3NDEKTSV4RRFFQ69G5FAV",
		SessionID:  "sess_01",
		Permission: "edit",
		Patterns:   []string{"/tmp/foo.go"},
		DecisionCh: ch,
	}
	if req.ChatID != 123 {
		t.Errorf("ChatID: got %d, want 123", req.ChatID)
	}
	if req.Permission != "edit" {
		t.Errorf("Permission: got %q, want edit", req.Permission)
	}
	if len(req.Patterns) != 1 {
		t.Errorf("Patterns: got %d, want 1", len(req.Patterns))
	}
	if req.DecisionCh == nil {
		t.Error("DecisionCh must not be nil")
	}
}

func TestQuestionRequestFields(t *testing.T) {
	req := hitl.QuestionRequest{
		ChatID:    456,
		ID:        "question_01ARZ3NDEKTSV4RRFFQ69G5FAV",
		SessionID: "sess_02",
		Questions: []hitl.Question{
			{
				Question: "Which test framework?",
				Header:   "Test",
				Options: []hitl.Option{
					{Label: "testify", Description: "Popular assertion library"},
					{Label: "stdlib", Description: "Built-in testing package"},
				},
				Multiple: false,
				Custom:   true,
			},
		},
	}
	if req.ChatID != 456 {
		t.Errorf("ChatID: got %d, want 456", req.ChatID)
	}
	if len(req.Questions) != 1 {
		t.Errorf("Questions: got %d, want 1", len(req.Questions))
	}
	q := req.Questions[0]
	if q.Question != "Which test framework?" {
		t.Errorf("Question text: got %q", q.Question)
	}
	if !q.Custom {
		t.Error("Custom must be true")
	}
	if len(q.Options) != 2 {
		t.Errorf("Options: got %d, want 2", len(q.Options))
	}
}

func TestOptionFields(t *testing.T) {
	opt := hitl.Option{Label: "yes", Description: "Confirm"}
	if opt.Label != "yes" {
		t.Errorf("Label: got %q, want yes", opt.Label)
	}
}
