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
	CWD        string
	SessionID  string
	Agent      model.AgentProfile
	ApprovalID string
	LogSink    ToolLogSink
}

type invocationContextKey struct{}

var ErrCWDRequired = fmt.Errorf("tools: cwd is required")

func WithInvocationContext(ctx context.Context, meta InvocationContext) context.Context {
	return context.WithValue(ctx, invocationContextKey{}, meta)
}

func InvocationContextFrom(ctx context.Context) (InvocationContext, bool) {
	meta, ok := ctx.Value(invocationContextKey{}).(InvocationContext)
	return meta, ok
}

func cwdFromContext(ctx context.Context) (string, error) {
	meta, ok := InvocationContextFrom(ctx)
	if !ok || meta.CWD == "" {
		return "", ErrCWDRequired
	}
	return meta.CWD, nil
}

func toolLogSinkFromContext(ctx context.Context) ToolLogSink {
	meta, ok := InvocationContextFrom(ctx)
	if !ok {
		return nil
	}
	return meta.LogSink
}
