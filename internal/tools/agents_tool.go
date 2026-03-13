package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/providers"
)

// agentOrchestrator is the minimal interface required by the three agent tools.
// Both opencode.Service and claudecode.Service satisfy this after the Task 6/7 extensions.
type agentOrchestrator interface {
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
}

// agentFor selects the appropriate orchestrator for the given agent kind.
// Returns nil for unknown kinds.
func agentFor(oc, cc agentOrchestrator, kind string) agentOrchestrator {
	switch kind {
	case "opencode":
		return oc
	case "claudecode":
		return cc
	}
	return nil
}

// --- spawn_agent ---

type spawnAgentTool struct {
	oc          agentOrchestrator
	cc          agentOrchestrator
	chatID      int64
	lifetimeCtx context.Context //nolint:containedctx
}

// NewSpawnAgentTool constructs the spawn_agent tool.
// lifetimeCtx should be the service's lifetime context (from gateway.Service.lifetimeCtx).
func NewSpawnAgentTool(oc, cc agentOrchestrator, chatID int64, lifetimeCtx context.Context) Tool {
	return &spawnAgentTool{oc: oc, cc: cc, chatID: chatID, lifetimeCtx: lifetimeCtx}
}

func (s *spawnAgentTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "spawn_agent",
		Description: "Dispatch a task to an AI coding agent asynchronously. Returns immediately.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind":   map[string]any{"type": "string", "enum": []string{"opencode", "claudecode"}},
				"prompt": map[string]any{"type": "string"},
			},
			"required": []string{"kind", "prompt"},
		},
	}
}

func (s *spawnAgentTool) Execute(_ context.Context, input map[string]any) ToolResult {
	kind, _ := input["kind"].(string)
	prompt, _ := input["prompt"].(string)
	if kind == "" || prompt == "" {
		return ToolResult{ForLLM: "spawn_agent: kind and prompt are required"}
	}
	agent := agentFor(s.oc, s.cc, kind)
	if agent == nil {
		return ToolResult{ForLLM: fmt.Sprintf("spawn_agent: unknown kind %q", kind)}
	}
	go func() {
		_ = agent.SubmitTask(s.lifetimeCtx, s.chatID, prompt)
	}()
	out, _ := json.Marshal(map[string]string{"status": "dispatched", "agent": kind})
	return ToolResult{
		ForLLM:  string(out),
		ForUser: fmt.Sprintf("Dispatched %s agent.", kind),
	}
}

// --- run_parallel ---

type runParallelTool struct {
	oc          agentOrchestrator
	cc          agentOrchestrator
	chatID      int64
	lifetimeCtx context.Context //nolint:containedctx
}

// NewRunParallelTool constructs the run_parallel tool.
func NewRunParallelTool(oc, cc agentOrchestrator, chatID int64, lifetimeCtx context.Context) Tool {
	return &runParallelTool{oc: oc, cc: cc, chatID: chatID, lifetimeCtx: lifetimeCtx}
}

func (r *runParallelTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "run_parallel",
		Description: "Dispatch multiple tasks to AI coding agents simultaneously.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tasks": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"kind":   map[string]any{"type": "string"},
							"prompt": map[string]any{"type": "string"},
						},
					},
				},
			},
			"required": []string{"tasks"},
		},
	}
}

type skippedTask struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

func (r *runParallelTool) Execute(_ context.Context, input map[string]any) ToolResult {
	tasksRaw, _ := input["tasks"].([]any)
	if len(tasksRaw) == 0 {
		return ToolResult{ForLLM: "run_parallel: tasks array is required"}
	}
	dispatched := 0
	skipped := []skippedTask{}
	for i, taskRaw := range tasksRaw {
		task, _ := taskRaw.(map[string]any)
		kind, _ := task["kind"].(string)
		prompt, _ := task["prompt"].(string)
		agent := agentFor(r.oc, r.cc, kind)
		if agent == nil {
			skipped = append(skipped, skippedTask{Index: i, Reason: fmt.Sprintf("unknown kind %q", kind)})
			continue
		}
		go func(a agentOrchestrator, p string) {
			_ = a.SubmitTask(r.lifetimeCtx, r.chatID, p)
		}(agent, prompt)
		dispatched++
	}
	result := map[string]any{"dispatched": dispatched}
	if len(skipped) > 0 {
		result["skipped"] = skipped
	}
	out, _ := json.Marshal(result)
	return ToolResult{
		ForLLM:  string(out),
		ForUser: fmt.Sprintf("Dispatched %d tasks in parallel.", dispatched),
	}
}

// --- chain_agents ---

type chainAgentsTool struct {
	oc     agentOrchestrator
	cc     agentOrchestrator
	chatID int64
}

// NewChainAgentsTool constructs the chain_agents tool.
func NewChainAgentsTool(oc, cc agentOrchestrator, chatID int64) Tool {
	return &chainAgentsTool{oc: oc, cc: cc, chatID: chatID}
}

func (c *chainAgentsTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "chain_agents",
		Description: "Run agents sequentially, passing each step's output to the next.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"kind":            map[string]any{"type": "string"},
							"prompt_template": map[string]any{"type": "string"},
						},
					},
				},
			},
			"required": []string{"steps"},
		},
	}
}

func (c *chainAgentsTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	stepsRaw, _ := input["steps"].([]any)
	if len(stepsRaw) == 0 {
		return ToolResult{ForLLM: "chain_agents: steps array is required"}
	}
	previousOutput := ""
	for i, stepRaw := range stepsRaw {
		step, _ := stepRaw.(map[string]any)
		kind, _ := step["kind"].(string)
		tmpl, _ := step["prompt_template"].(string)
		prompt := strings.ReplaceAll(tmpl, "{{previous_output}}", previousOutput)
		agent := agentFor(c.oc, c.cc, kind)
		if agent == nil {
			return ToolResult{ForLLM: fmt.Sprintf("chain_agents: unknown kind %q at step %d", kind, i+1)}
		}
		output, err := agent.SubmitTaskWithResult(ctx, c.chatID, prompt)
		if err != nil {
			return ToolResult{ForLLM: fmt.Sprintf("chain_agents: aborted at step %d: %v", i+1, err)}
		}
		previousOutput = output
	}
	if previousOutput == "" {
		return ToolResult{ForLLM: `{"status":"completed","output":""}`}
	}
	return ToolResult{ForLLM: previousOutput}
}
