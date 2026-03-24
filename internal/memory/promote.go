package memory

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
)

// authorizedScopes lists the scope levels ordered from narrowest to broadest.
var authorizedScopes = map[string]int{
	"local": 0,
	"team":  1,
}

// AuthorizeEscalation returns an error if promoting from currentScope to
// targetScope is not permitted. The run engine calls this before invoking
// WriteFact with a higher scope; the memory store does not call it.
func AuthorizeEscalation(currentScope, targetScope string) error {
	curLevel, curOK := authorizedScopes[currentScope]
	tgtLevel, tgtOK := authorizedScopes[targetScope]
	if !curOK {
		return fmt.Errorf("memory: unknown current scope %q", currentScope)
	}
	if !tgtOK {
		return fmt.Errorf("memory: unknown target scope %q", targetScope)
	}
	if tgtLevel <= curLevel {
		return fmt.Errorf("memory: scope escalation from %q to %q is not an escalation", currentScope, targetScope)
	}
	return nil
}

func (s *Store) PromoteCandidate(ctx context.Context, candidate model.MemoryCandidate) error {
	if candidate.AgentID == "" {
		return fmt.Errorf("memory: promote: agent_id required")
	}
	if candidate.Scope == "" {
		return fmt.Errorf("memory: promote: scope required")
	}
	if candidate.Content == "" {
		return fmt.Errorf("memory: promote: content required")
	}
	if candidate.DedupeKey == "" {
		return fmt.Errorf("memory: promote: dedupe_key required")
	}

	id := memGenerateID()
	err := s.WriteFact(ctx, model.MemoryItem{
		ID:         id,
		AgentID:    candidate.AgentID,
		Scope:      candidate.Scope,
		Content:    candidate.Content,
		Source:     "model",
		Provenance: candidate.Provenance,
		Confidence: candidate.Confidence,
		DedupeKey:  candidate.DedupeKey,
	})
	if err != nil {
		return fmt.Errorf("memory: promote write: %w", err)
	}

	if candidate.ConversationID != "" {
		err = s.convStore.AppendEvent(ctx, model.Event{
			ID:             memGenerateID(),
			ConversationID: candidate.ConversationID,
			Kind:           "memory_promoted",
			PayloadJSON: []byte(fmt.Sprintf(
				`{"memory_id":"%s","agent_id":"%s","scope":"%s","dedupe_key":"%s"}`,
				id, candidate.AgentID, candidate.Scope, candidate.DedupeKey,
			)),
		})
		if err != nil {
			return fmt.Errorf("memory: promote event: %w", err)
		}
	}

	return nil
}
