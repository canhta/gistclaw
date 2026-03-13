package tools_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/tools"
)

// mockOrchestrator records calls and returns configured responses.
type mockOrchestrator struct {
	submitErr           error
	submitWithResultOut string
	submitWithResultErr error
	submitCalls         atomic.Int32
	withResultCalls     atomic.Int32
}

func (m *mockOrchestrator) SubmitTask(_ context.Context, _ int64, _ string) error {
	m.submitCalls.Add(1)
	return m.submitErr
}
func (m *mockOrchestrator) SubmitTaskWithResult(_ context.Context, _ int64, _ string) (string, error) {
	m.withResultCalls.Add(1)
	time.Sleep(1 * time.Millisecond) // simulate brief work
	return m.submitWithResultOut, m.submitWithResultErr
}

func TestSpawnAgent_ReturnsImmediately(t *testing.T) {
	oc := &mockOrchestrator{}
	tool := tools.NewSpawnAgentTool(oc, oc, 123, context.Background())
	result := tool.Execute(context.Background(), map[string]any{
		"kind":   "opencode",
		"prompt": "do something",
	})
	if result.ForLLM == "" {
		t.Error("ForLLM should not be empty")
	}
	// Goroutine should eventually call SubmitTask.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if oc.submitCalls.Load() >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("expected agent dispatched; submitCalls=%d", oc.submitCalls.Load())
}

func TestRunParallel_DispatchesAll(t *testing.T) {
	oc := &mockOrchestrator{}
	cc := &mockOrchestrator{}
	tool := tools.NewRunParallelTool(oc, cc, 123, context.Background())
	result := tool.Execute(context.Background(), map[string]any{
		"tasks": []any{
			map[string]any{"kind": "opencode", "prompt": "task 1"},
			map[string]any{"kind": "claudecode", "prompt": "task 2"},
		},
	})
	if result.ForLLM == "" {
		t.Error("ForLLM should not be empty")
	}
	// Both goroutines should eventually call SubmitTask.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if oc.submitCalls.Load() >= 1 && cc.submitCalls.Load() >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("expected both agents dispatched; oc=%d cc=%d",
		oc.submitCalls.Load(), cc.submitCalls.Load())
}

func TestChainAgents_ThreadsOutput(t *testing.T) {
	oc := &mockOrchestrator{submitWithResultOut: "step1 output"}
	tool := tools.NewChainAgentsTool(oc, oc, 123)
	result := tool.Execute(context.Background(), map[string]any{
		"steps": []any{
			map[string]any{"kind": "opencode", "prompt_template": "start"},
			map[string]any{"kind": "opencode", "prompt_template": "continue with: {{previous_output}}"},
		},
	})
	if !strings.Contains(result.ForLLM, "step1 output") {
		t.Errorf("chain output should contain step1 output: %q", result.ForLLM)
	}
}

func TestChainAgents_AbortOnError(t *testing.T) {
	oc := &mockOrchestrator{submitWithResultErr: errors.New("agent failed")}
	tool := tools.NewChainAgentsTool(oc, oc, 123)
	result := tool.Execute(context.Background(), map[string]any{
		"steps": []any{
			map[string]any{"kind": "opencode", "prompt_template": "start"},
		},
	})
	if !strings.Contains(result.ForLLM, "aborted") && !strings.Contains(result.ForLLM, "error") {
		t.Errorf("chain should report error on step failure: %q", result.ForLLM)
	}
}
