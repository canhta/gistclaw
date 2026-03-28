package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

type CollaborationHandlers struct {
	Spawn func(context.Context, SessionSpawnRequest) (SessionSpawnResult, error)
}

type SessionSpawnRequest struct {
	ControllerSessionID string
	AgentID             string
	Prompt              string
}

type SessionSpawnResult struct {
	RunID     string
	SessionID string
	AgentID   string
	Status    model.RunStatus
	Output    string
}

func RegisterCollaborationTools(reg *Registry, handlers CollaborationHandlers) {
	if reg == nil {
		return
	}
	if handlers.Spawn != nil {
		reg.Register(&SessionSpawnTool{spawn: handlers.Spawn})
	}
}

type SessionSpawnTool struct {
	spawn func(context.Context, SessionSpawnRequest) (SessionSpawnResult, error)
}

func (t *SessionSpawnTool) Name() string { return "session_spawn" }

func (t *SessionSpawnTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Spawn a specialist agent run and return its result.",
		InputSchemaJSON: `{"type":"object","properties":{"agent_id":{"type":"string"},"prompt":{"type":"string"}},"required":["agent_id","prompt"],"additionalProperties":false}`,
		Family:          model.ToolFamilyDelegate,
		Risk:            model.RiskLow,
	}
}

func (t *SessionSpawnTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	if t == nil || t.spawn == nil {
		return model.ToolResult{}, fmt.Errorf("session_spawn: handler is required")
	}
	meta, ok := InvocationContextFrom(ctx)
	if !ok || strings.TrimSpace(meta.SessionID) == "" {
		return model.ToolResult{}, fmt.Errorf("session_spawn: caller session is required")
	}

	var input struct {
		AgentID string `json:"agent_id"`
		Prompt  string `json:"prompt"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("session_spawn: decode input: %w", err)
	}
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.AgentID == "" {
		return model.ToolResult{}, fmt.Errorf("session_spawn: agent_id is required")
	}
	if input.Prompt == "" {
		return model.ToolResult{}, fmt.Errorf("session_spawn: prompt is required")
	}
	if err := validateDelegationTarget(meta, input.AgentID); err != nil {
		return model.ToolResult{}, err
	}

	result, err := t.spawn(ctx, SessionSpawnRequest{
		ControllerSessionID: meta.SessionID,
		AgentID:             input.AgentID,
		Prompt:              input.Prompt,
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
		return model.ToolResult{}, fmt.Errorf("session_spawn: encode output: %w", err)
	}
	return model.ToolResult{Output: string(payload)}, nil
}

func validateDelegationTarget(meta InvocationContext, targetAgentID string) error {
	target, ok := meta.Specialists[targetAgentID]
	if !ok {
		return fmt.Errorf("session_spawn: %s is not a known specialist", targetAgentID)
	}
	kind, ok := delegationKindForBaseProfile(target.BaseProfile)
	if !ok {
		return fmt.Errorf("session_spawn: %s is not a delegatable specialist", targetAgentID)
	}
	if !containsDelegationKind(meta.Agent.DelegationKinds, kind) {
		return fmt.Errorf("session_spawn: %s cannot delegate %s work to %s", meta.Agent.AgentID, kind, targetAgentID)
	}
	return nil
}

func delegationKindForBaseProfile(profile model.BaseProfile) (model.DelegationKind, bool) {
	switch profile {
	case model.BaseProfileResearch:
		return model.DelegationKindResearch, true
	case model.BaseProfileWrite:
		return model.DelegationKindWrite, true
	case model.BaseProfileReview:
		return model.DelegationKindReview, true
	case model.BaseProfileVerify:
		return model.DelegationKindVerify, true
	default:
		return "", false
	}
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
