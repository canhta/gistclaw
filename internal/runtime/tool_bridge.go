package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	recommendationpkg "github.com/canhta/gistclaw/internal/runtime/recommendation"
	"github.com/canhta/gistclaw/internal/tools"
)

func (r *Runtime) DelegateTaskTool(ctx context.Context, req tools.DelegateTaskRequest) (tools.DelegationResult, error) {
	controllerSession, controllerRun, err := r.loadSessionRun(ctx, req.ControllerSessionID)
	if err != nil {
		return tools.DelegationResult{}, err
	}
	_, specialists, err := r.agentContextForRun(ctx, controllerRun.ID, controllerRun.AgentID)
	if err != nil {
		return tools.DelegationResult{}, err
	}
	targetAgentID, err := selectSpecialistForKind(specialists, req.Kind, req.Objective)
	if err != nil {
		return tools.DelegationResult{}, err
	}
	return r.delegateToAgent(ctx, controllerSession.ID, targetAgentID, req.Objective)
}

func (r *Runtime) delegateToAgent(
	ctx context.Context,
	controllerSessionID string,
	targetAgentID string,
	objective string,
) (tools.DelegationResult, error) {
	run, err := r.Spawn(ctx, SpawnCommand{
		ControllerSessionID: controllerSessionID,
		AgentID:             targetAgentID,
		Prompt:              objective,
	})
	if err != nil {
		return tools.DelegationResult{}, err
	}
	output, err := r.latestAssistantMessage(ctx, run.SessionID)
	if err != nil {
		return tools.DelegationResult{}, err
	}
	if strings.TrimSpace(output) == "" && isTerminalRunStatus(run.Status) {
		output, err = r.childTerminalMessage(ctx, run)
		if err != nil {
			return tools.DelegationResult{}, err
		}
	}
	return tools.DelegationResult{
		RunID:     run.ID,
		SessionID: run.SessionID,
		AgentID:   run.AgentID,
		Status:    run.Status,
		Output:    output,
	}, nil
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

func selectSpecialistForKind(
	specialists map[string]model.AgentProfile,
	kind model.DelegationKind,
	objective string,
) (string, error) {
	targetProfile, ok := delegationKindBaseProfile(kind)
	if !ok {
		return "", fmt.Errorf("runtime: unsupported delegation kind %q", kind)
	}
	descriptor := recommendationpkg.AnalyzeObjective(objective)
	candidates := make([]specialistCandidate, 0, len(specialists))
	for agentID, specialist := range specialists {
		if specialist.BaseProfile == targetProfile {
			candidates = append(candidates, specialistCandidate{
				agentID: agentID,
				score:   specialistScore(descriptor, specialist),
			})
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("runtime: no specialist available for %s work", kind)
	}
	slices.SortFunc(candidates, func(a, b specialistCandidate) int {
		switch {
		case a.score > b.score:
			return -1
		case a.score < b.score:
			return 1
		case a.agentID < b.agentID:
			return -1
		case a.agentID > b.agentID:
			return 1
		default:
			return 0
		}
	})
	return candidates[0].agentID, nil
}

type specialistCandidate struct {
	agentID string
	score   int
}

func specialistScore(descriptor recommendationpkg.ObjectiveDescriptor, specialist model.AgentProfile) int {
	score := 0
	text := " " + descriptor.Text + " "
	for _, specialty := range specialist.Specialties {
		normalized := strings.TrimSpace(strings.ToLower(specialty))
		if normalized == "" {
			continue
		}
		if strings.Contains(text, " "+normalized+" ") || strings.Contains(text, normalized) {
			score += 4
			continue
		}
		for _, keyword := range descriptor.Keywords {
			if keyword == normalized {
				score += 2
			}
		}
	}
	return score
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
