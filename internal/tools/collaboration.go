package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

type CollaborationHandlers struct {
	DelegateTask func(context.Context, DelegateTaskRequest) (DelegationResult, error)
}

type DelegationResult struct {
	RunID     string
	SessionID string
	AgentID   string
	Status    model.RunStatus
	Output    string
}

type DelegateTaskRequest struct {
	ControllerSessionID string
	Kind                model.DelegationKind
	Objective           string
}

type DelegateTaskTool struct {
	delegate func(context.Context, DelegateTaskRequest) (DelegationResult, error)
}

func RegisterCollaborationTools(reg *Registry, handlers CollaborationHandlers) {
	if reg == nil {
		return
	}
	if handlers.DelegateTask != nil {
		reg.Register(&DelegateTaskTool{delegate: handlers.DelegateTask})
	}
}

func (t *DelegateTaskTool) Name() string { return "delegate_task" }

func (t *DelegateTaskTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Delegate a specialist task by kind and let the runtime choose the matching worker.",
		InputSchemaJSON: `{"type":"object","properties":{"kind":{"type":"string","enum":["research","write","review","verify"]},"objective":{"type":"string"}},"required":["kind","objective"],"additionalProperties":false}`,
		Family:          model.ToolFamilyDelegate,
		Intents:         []model.ToolIntent{model.ToolIntentDelegate},
		Risk:            model.RiskLow,
	}
}

func (t *DelegateTaskTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	if t == nil || t.delegate == nil {
		return model.ToolResult{}, fmt.Errorf("delegate_task: handler is required")
	}
	meta, ok := InvocationContextFrom(ctx)
	if !ok || strings.TrimSpace(meta.SessionID) == "" {
		return model.ToolResult{}, fmt.Errorf("delegate_task: caller session is required")
	}

	var input struct {
		Kind      model.DelegationKind `json:"kind"`
		Objective string               `json:"objective"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("delegate_task: decode input: %w", err)
	}
	input.Kind = model.DelegationKind(strings.TrimSpace(string(input.Kind)))
	input.Objective = strings.TrimSpace(input.Objective)
	if input.Kind == "" {
		return model.ToolResult{}, fmt.Errorf("delegate_task: kind is required")
	}
	if !model.IsValidDelegationKind(string(input.Kind)) {
		return model.ToolResult{}, fmt.Errorf("delegate_task: unsupported kind %q", input.Kind)
	}
	if input.Objective == "" {
		return model.ToolResult{}, fmt.Errorf("delegate_task: objective is required")
	}
	if err := validateDelegationKind(meta, "delegate_task", input.Kind); err != nil {
		return model.ToolResult{}, err
	}

	result, err := t.delegate(ctx, DelegateTaskRequest{
		ControllerSessionID: meta.SessionID,
		Kind:                input.Kind,
		Objective:           input.Objective,
	})
	if err != nil {
		return model.ToolResult{}, err
	}
	payload, err := json.Marshal(map[string]any{
		"run_id":     result.RunID,
		"session_id": result.SessionID,
		"agent_id":   result.AgentID,
		"status":     result.Status,
		"output":     result.Output,
	})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("delegate_task: encode output: %w", err)
	}
	return model.ToolResult{Output: string(payload)}, nil
}

func validateDelegationKind(meta InvocationContext, toolName string, kind model.DelegationKind) error {
	if !containsDelegationKind(meta.Agent.DelegationKinds, kind) {
		return fmt.Errorf("%s: %s cannot delegate %s work", toolName, meta.Agent.AgentID, kind)
	}
	return nil
}

func containsDelegationKind(values []model.DelegationKind, want model.DelegationKind) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
