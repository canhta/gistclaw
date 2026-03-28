package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/authority"
)

var ErrRemoteConnectorUnsafeAuthority = fmt.Errorf("runtime: remote connectors cannot run with auto_approve + elevated")

func (r *Runtime) resolveRuntimeAuthorityJSON(ctx context.Context, raw []byte) ([]byte, error) {
	env, err := r.loadRuntimeAuthorityDefaults(ctx)
	if err != nil {
		return nil, err
	}
	override, err := decodeAuthorityOverride(raw)
	if err != nil {
		return nil, err
	}
	if override.ApprovalMode != "" {
		env.ApprovalMode = override.ApprovalMode
	}
	if override.HostAccessMode != "" {
		env.HostAccessMode = override.HostAccessMode
	}
	if override.Capabilities != nil {
		env.Capabilities = override.Capabilities
	}
	env = authority.NormalizeEnvelope(env)
	normalized, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal authority envelope: %w", err)
	}
	return normalized, nil
}

func (r *Runtime) loadRuntimeAuthorityDefaults(ctx context.Context) (authority.Envelope, error) {
	env := authority.NormalizeEnvelope(authority.Envelope{})

	approvalMode, err := r.lookupRuntimeSetting(ctx, "approval_mode")
	if err != nil {
		return authority.Envelope{}, err
	}
	if approvalMode != "" {
		env.ApprovalMode = authority.ApprovalMode(approvalMode)
	}

	hostAccessMode, err := r.lookupRuntimeSetting(ctx, "host_access_mode")
	if err != nil {
		return authority.Envelope{}, err
	}
	if hostAccessMode != "" {
		env.HostAccessMode = authority.HostAccessMode(hostAccessMode)
	}

	return authority.NormalizeEnvelope(env), nil
}

func (r *Runtime) lookupRuntimeSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.store.RawDB().QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load setting %q: %w", key, err)
	}
	return strings.TrimSpace(value), nil
}

func decodeAuthorityOverride(raw []byte) (authority.Envelope, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return authority.Envelope{}, nil
	}
	var env authority.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return authority.Envelope{}, fmt.Errorf("decode authority override: %w", err)
	}
	return env, nil
}

func (r *Runtime) enforceConversationAuthority(ctx context.Context, conversationID, sourceConnectorID string, env authority.Envelope) error {
	if env.ApprovalMode != authority.ApprovalModeAutoApprove || env.HostAccessMode != authority.HostAccessModeElevated {
		return nil
	}
	connectorID := strings.TrimSpace(sourceConnectorID)
	if connectorID == "" {
		var err error
		connectorID, err = r.conversationConnectorID(ctx, conversationID)
		if err != nil {
			return err
		}
	}
	if r.isRemoteConnector(connectorID) {
		return ErrRemoteConnectorUnsafeAuthority
	}
	return nil
}

func (r *Runtime) conversationConnectorID(ctx context.Context, conversationID string) (string, error) {
	if strings.TrimSpace(conversationID) == "" {
		return "", nil
	}
	var normalizedKey string
	err := r.store.RawDB().QueryRowContext(ctx, "SELECT key FROM conversations WHERE id = ?", conversationID).Scan(&normalizedKey)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load conversation connector: %w", err)
	}
	return parseConversationConnectorID(normalizedKey), nil
}

func parseConversationConnectorID(normalizedKey string) string {
	part := normalizedKey
	if idx := strings.IndexByte(part, ':'); idx >= 0 {
		part = part[:idx]
	}
	return strings.ReplaceAll(part, "%3A", ":")
}
