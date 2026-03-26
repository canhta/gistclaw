package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type ToolLogRecord struct {
	Stream     string
	Text       string
	OccurredAt time.Time
}

type ToolLogSink interface {
	Record(context.Context, ToolLogRecord) error
}

type InvocationContext struct {
	WorkspaceRoot string
	SessionID     string
	Agent         model.AgentProfile
	ApprovalID    string
	LogSink       ToolLogSink
}

type invocationContextKey struct{}

var ErrWorkspaceRequired = fmt.Errorf("tools: workspace root is required")

func WithInvocationContext(ctx context.Context, meta InvocationContext) context.Context {
	return context.WithValue(ctx, invocationContextKey{}, meta)
}

func InvocationContextFrom(ctx context.Context) (InvocationContext, bool) {
	meta, ok := ctx.Value(invocationContextKey{}).(InvocationContext)
	return meta, ok
}

func workspaceRootFromContext(ctx context.Context) (string, error) {
	meta, ok := InvocationContextFrom(ctx)
	if !ok || meta.WorkspaceRoot == "" {
		return "", ErrWorkspaceRequired
	}
	return meta.WorkspaceRoot, nil
}

func toolLogSinkFromContext(ctx context.Context) ToolLogSink {
	meta, ok := InvocationContextFrom(ctx)
	if !ok {
		return nil
	}
	return meta.LogSink
}
