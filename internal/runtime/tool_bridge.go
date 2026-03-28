package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/tools"
)

func (r *Runtime) SpawnTool(ctx context.Context, req tools.SessionSpawnRequest) (tools.SessionSpawnResult, error) {
	run, err := r.Spawn(ctx, SpawnCommand{
		ControllerSessionID: req.ControllerSessionID,
		AgentID:             req.AgentID,
		Prompt:              req.Prompt,
	})
	if err != nil {
		return tools.SessionSpawnResult{}, err
	}
	output, err := r.latestAssistantMessage(ctx, run.SessionID)
	if err != nil {
		return tools.SessionSpawnResult{}, err
	}
	if strings.TrimSpace(output) == "" && isTerminalRunStatus(run.Status) {
		output, err = r.childTerminalMessage(ctx, run)
		if err != nil {
			return tools.SessionSpawnResult{}, err
		}
	}
	return tools.SessionSpawnResult{
		RunID:     run.ID,
		SessionID: run.SessionID,
		AgentID:   run.AgentID,
		Status:    run.Status,
		Output:    output,
	}, nil
}

func (r *Runtime) DelegateTaskTool(ctx context.Context, req tools.DelegateTaskRequest) (tools.SessionSpawnResult, error) {
	controllerSession, controllerRun, err := r.loadSessionRun(ctx, req.ControllerSessionID)
	if err != nil {
		return tools.SessionSpawnResult{}, err
	}
	_, specialists, err := r.agentContextForRun(ctx, controllerRun.ID, controllerRun.AgentID)
	if err != nil {
		return tools.SessionSpawnResult{}, err
	}
	targetAgentID, err := selectSpecialistForKind(specialists, req.Kind)
	if err != nil {
		return tools.SessionSpawnResult{}, err
	}
	return r.SpawnTool(ctx, tools.SessionSpawnRequest{
		ControllerSessionID: controllerSession.ID,
		AgentID:             targetAgentID,
		Prompt:              req.Objective,
	})
}

func (r *Runtime) latestAssistantMessage(ctx context.Context, sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", nil
	}
	var body string
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT body
		 FROM session_messages
		 WHERE session_id = ? AND kind = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		sessionID,
		model.MessageAssistant,
	).Scan(&body)
	if err == nil {
		return body, nil
	}
	if err == sql.ErrNoRows {
		return "", nil
	}
	return "", fmt.Errorf("runtime: load latest assistant message: %w", err)
}

func selectSpecialistForKind(specialists map[string]model.AgentProfile, kind model.DelegationKind) (string, error) {
	targetProfile, ok := delegationKindBaseProfile(kind)
	if !ok {
		return "", fmt.Errorf("runtime: unsupported delegation kind %q", kind)
	}
	agentIDs := make([]string, 0, len(specialists))
	for agentID, specialist := range specialists {
		if specialist.BaseProfile == targetProfile {
			agentIDs = append(agentIDs, agentID)
		}
	}
	if len(agentIDs) == 0 {
		return "", fmt.Errorf("runtime: no specialist available for %s work", kind)
	}
	slices.Sort(agentIDs)
	return agentIDs[0], nil
}

func delegationKindBaseProfile(kind model.DelegationKind) (model.BaseProfile, bool) {
	switch kind {
	case model.DelegationKindResearch:
		return model.BaseProfileResearch, true
	case model.DelegationKindWrite:
		return model.BaseProfileWrite, true
	case model.DelegationKindReview:
		return model.BaseProfileReview, true
	case model.DelegationKindVerify:
		return model.BaseProfileVerify, true
	default:
		return "", false
	}
}
