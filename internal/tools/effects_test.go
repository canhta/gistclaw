package tools

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestClassifyToolCall_UsesDeclaredEffectClassifier(t *testing.T) {
	spec := model.ToolSpec{
		Name:             "remote_exec",
		Family:           model.ToolFamilyRepoWrite,
		SideEffect:       effectExecWrite,
		EffectClassifier: model.ToolEffectClassifierShellCommand,
	}

	if got := classifyToolCall(spec, []byte(`{"command":"git status --short"}`)); got != effectExecRead {
		t.Fatalf("expected git status to classify as %q, got %q", effectExecRead, got)
	}
}
