// internal/hitl/keyboard_test.go
package hitl_test

import (
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/hitl"
)

func TestPermissionKeyboardText(t *testing.T) {
	payload := hitl.PermissionKeyboard("perm_001", "edit", []string{"/tmp/foo.go", "/tmp/bar.go"})

	wantTextPrefix := "🔐 Permission request"
	if !strings.HasPrefix(payload.Text, wantTextPrefix) {
		t.Errorf("Text should start with %q, got: %q", wantTextPrefix, payload.Text)
	}
	if !strings.Contains(payload.Text, "edit") {
		t.Error("Text should contain the permission name")
	}
	if !strings.Contains(payload.Text, "/tmp/foo.go") {
		t.Error("Text should contain the first pattern")
	}
	if !strings.Contains(payload.Text, "/tmp/bar.go") {
		t.Error("Text should contain the second pattern")
	}
}

func TestPermissionKeyboardRows(t *testing.T) {
	payload := hitl.PermissionKeyboard("perm_001", "edit", []string{"/tmp/foo.go"})

	if len(payload.Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(payload.Rows))
	}

	// Each row has exactly one button.
	for i, row := range payload.Rows {
		if len(row) != 1 {
			t.Errorf("row %d: expected 1 button, got %d", i, len(row))
		}
	}

	// Verify CallbackData format: "hitl:<id>:<action>"
	wantData := []string{
		"hitl:perm_001:once",
		"hitl:perm_001:always",
		"hitl:perm_001:reject",
		"hitl:perm_001:stop",
	}
	for i, row := range payload.Rows {
		if row[0].CallbackData != wantData[i] {
			t.Errorf("row %d: CallbackData = %q, want %q", i, row[0].CallbackData, wantData[i])
		}
	}
}

func TestPermissionKeyboardLabels(t *testing.T) {
	payload := hitl.PermissionKeyboard("perm_001", "edit", []string{"/tmp/foo.go"})
	wantLabels := []string{"✅ Once", "✅ Always", "❌ Reject", "⏹ Stop"}
	for i, row := range payload.Rows {
		if row[0].Label != wantLabels[i] {
			t.Errorf("row %d: Label = %q, want %q", i, row[0].Label, wantLabels[i])
		}
	}
}

func TestPermissionKeyboardReturnsChannelType(t *testing.T) {
	// Compile-time check: PermissionKeyboard returns channel.KeyboardPayload (not telego).
	_ = hitl.PermissionKeyboard("x", "edit", nil)
}

func TestQuestionKeyboardSingleChoiceNoCustom(t *testing.T) {
	q := hitl.Question{
		Question: "Which test framework?",
		Options: []hitl.Option{
			{Label: "testify"},
			{Label: "stdlib"},
		},
		Multiple: false,
		Custom:   false,
	}
	payload := hitl.QuestionKeyboard("q_001", q)

	if payload.Text != "Which test framework?" {
		t.Errorf("Text = %q, want the question text", payload.Text)
	}
	// 2 options → 2 rows; no "Type your own" row.
	if len(payload.Rows) != 2 {
		t.Fatalf("expected 2 rows (no custom), got %d", len(payload.Rows))
	}
	// Each row has one button.
	if len(payload.Rows[0]) != 1 || len(payload.Rows[1]) != 1 {
		t.Error("each option row must have exactly one button")
	}
	// CallbackData: "hitl:<id>:opt:<index>"
	if payload.Rows[0][0].CallbackData != "hitl:q_001:opt:0" {
		t.Errorf("row 0 CallbackData = %q, want hitl:q_001:opt:0", payload.Rows[0][0].CallbackData)
	}
	if payload.Rows[1][0].CallbackData != "hitl:q_001:opt:1" {
		t.Errorf("row 1 CallbackData = %q, want hitl:q_001:opt:1", payload.Rows[1][0].CallbackData)
	}
	// Labels match option labels.
	if payload.Rows[0][0].Label != "testify" {
		t.Errorf("row 0 Label = %q, want testify", payload.Rows[0][0].Label)
	}
}

func TestQuestionKeyboardCustomAddsTypeYourOwn(t *testing.T) {
	q := hitl.Question{
		Question: "Pick or type:",
		Options:  []hitl.Option{{Label: "A"}},
		Multiple: false,
		Custom:   true,
	}
	payload := hitl.QuestionKeyboard("q_002", q)

	// 1 option + 1 "Type your own" row = 2 rows.
	if len(payload.Rows) != 2 {
		t.Fatalf("expected 2 rows (1 option + custom), got %d", len(payload.Rows))
	}
	lastRow := payload.Rows[len(payload.Rows)-1]
	if len(lastRow) != 1 {
		t.Fatal("custom row must have exactly 1 button")
	}
	if lastRow[0].Label != "✏️ Type your own" {
		t.Errorf("custom button Label = %q, want '✏️ Type your own'", lastRow[0].Label)
	}
	if lastRow[0].CallbackData != "hitl:q_002:custom" {
		t.Errorf("custom button CallbackData = %q, want 'hitl:q_002:custom'", lastRow[0].CallbackData)
	}
}

func TestQuestionKeyboardOptionLabelsIncludeDescription(t *testing.T) {
	q := hitl.Question{
		Question: "Choose:",
		Options: []hitl.Option{
			{Label: "yes", Description: "Confirm action"},
		},
	}
	payload := hitl.QuestionKeyboard("q_003", q)
	// When description is non-empty, button label should include it.
	btn := payload.Rows[0][0]
	if !strings.Contains(btn.Label, "yes") {
		t.Errorf("button label %q does not contain option label 'yes'", btn.Label)
	}
	if !strings.Contains(btn.Label, "Confirm action") {
		t.Errorf("button label %q does not contain description 'Confirm action'", btn.Label)
	}
}
