package tools

import (
	"context"
	"fmt"
)

type InvocationContext struct {
	WorkspaceRoot string
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
