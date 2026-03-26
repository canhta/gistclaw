package runtime

import (
	"context"
	"database/sql"
	"fmt"
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
