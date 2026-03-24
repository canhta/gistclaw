package runtime

import (
	"context"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
)

func (r *Runtime) ListSessions(ctx context.Context, conversationID string, limit int) ([]model.Session, error) {
	return sessions.NewService(r.store, r.convStore).ListConversationSessions(ctx, conversationID, limit)
}

func (r *Runtime) SessionHistory(ctx context.Context, sessionID string, limit int) (model.Session, []model.SessionMessage, error) {
	return sessions.NewService(r.store, r.convStore).LoadSessionMailbox(ctx, sessionID, limit)
}
