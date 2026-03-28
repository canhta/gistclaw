package capabilities

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type DirectoryListRequest struct {
	ConnectorID string `json:"connector_id"`
	Scope       string `json:"scope,omitempty"`
	Query       string `json:"query,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type PresenceMode string

const PresenceModeTyping PresenceMode = "typing"

type PresenceEmitRequest struct {
	ConnectorID    string       `json:"connector_id"`
	ConversationID string       `json:"conversation_id,omitempty"`
	ThreadID       string       `json:"thread_id"`
	ThreadType     string       `json:"thread_type,omitempty"`
	Mode           PresenceMode `json:"mode"`
}

type PresencePolicy struct {
	StartupDelay           time.Duration `json:"startup_delay,omitempty"`
	KeepaliveInterval      time.Duration `json:"keepalive_interval,omitempty"`
	MaxDuration            time.Duration `json:"max_duration,omitempty"`
	MaxConsecutiveFailures int           `json:"max_consecutive_failures,omitempty"`
	SupportsStop           bool          `json:"supports_stop,omitempty"`
}

type InboxListRequest struct {
	ConnectorID string `json:"connector_id"`
	Scope       string `json:"scope,omitempty"`
	Query       string `json:"query,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	UnreadOnly  bool   `json:"unread_only,omitempty"`
}

type InboxEntry struct {
	ThreadID           string            `json:"thread_id"`
	ThreadType         string            `json:"thread_type,omitempty"`
	Title              string            `json:"title"`
	Subtitle           string            `json:"subtitle,omitempty"`
	UnreadCount        int               `json:"unread_count,omitempty"`
	IsUnread           bool              `json:"is_unread,omitempty"`
	LastMessagePreview string            `json:"last_message_preview,omitempty"`
	LastMessageAt      time.Time         `json:"last_message_at,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

type InboxListResult struct {
	ConnectorID string       `json:"connector_id"`
	Scope       string       `json:"scope,omitempty"`
	Entries     []InboxEntry `json:"entries"`
}

type InboxUpdateRequest struct {
	ConnectorID string `json:"connector_id"`
	ThreadID    string `json:"thread_id"`
	ThreadType  string `json:"thread_type,omitempty"`
	Action      string `json:"action"`
}

type InboxUpdateResult struct {
	ConnectorID string `json:"connector_id"`
	ThreadID    string `json:"thread_id"`
	ThreadType  string `json:"thread_type,omitempty"`
	Action      string `json:"action"`
	Applied     bool   `json:"applied"`
	Summary     string `json:"summary,omitempty"`
}

type DirectoryEntry struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Subtitle string            `json:"subtitle,omitempty"`
	Kind     string            `json:"kind,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type DirectoryListResult struct {
	ConnectorID string           `json:"connector_id"`
	Scope       string           `json:"scope,omitempty"`
	Entries     []DirectoryEntry `json:"entries"`
}

type TargetResolveRequest struct {
	ConnectorID string `json:"connector_id"`
	Query       string `json:"query"`
	Scope       string `json:"scope,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type TargetMatch struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Subtitle string            `json:"subtitle,omitempty"`
	Kind     string            `json:"kind,omitempty"`
	Score    float64           `json:"score,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type TargetResolveResult struct {
	ConnectorID string        `json:"connector_id"`
	Query       string        `json:"query"`
	Matches     []TargetMatch `json:"matches"`
}

type SendRequest struct {
	ConnectorID string            `json:"connector_id"`
	TargetID    string            `json:"target_id"`
	TargetType  string            `json:"target_type,omitempty"`
	Message     string            `json:"message"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type SendResult struct {
	ConnectorID string `json:"connector_id"`
	TargetID    string `json:"target_id"`
	TargetType  string `json:"target_type,omitempty"`
	Accepted    bool   `json:"accepted"`
	Summary     string `json:"summary,omitempty"`
}

type StatusRequest struct {
	ConnectorID string `json:"connector_id,omitempty"`
}

type ConnectorStatus struct {
	ConnectorID      string                     `json:"connector_id"`
	State            model.ConnectorHealthState `json:"state"`
	Summary          string                     `json:"summary,omitempty"`
	CheckedAt        time.Time                  `json:"checked_at,omitempty"`
	RestartSuggested bool                       `json:"restart_suggested,omitempty"`
}

type StatusResult struct {
	Connectors []ConnectorStatus `json:"connectors"`
}

type AppActionRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type AppActionResult struct {
	Name    string         `json:"name"`
	Summary string         `json:"summary,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

type DirectoryAdapter interface {
	CapabilityListDirectory(context.Context, DirectoryListRequest) (DirectoryListResult, error)
}

type PresenceAdapter interface {
	CapabilityPresencePolicy(context.Context) PresencePolicy
	CapabilityEmitPresence(context.Context, PresenceEmitRequest) error
}

type InboxAdapter interface {
	CapabilityListInbox(context.Context, InboxListRequest) (InboxListResult, error)
}

type InboxUpdater interface {
	CapabilityUpdateInbox(context.Context, InboxUpdateRequest) (InboxUpdateResult, error)
}

type TargetResolver interface {
	CapabilityResolveTarget(context.Context, TargetResolveRequest) (TargetResolveResult, error)
}

type Sender interface {
	CapabilitySend(context.Context, SendRequest) (SendResult, error)
}

type StatusAdapter interface {
	CapabilityStatus(context.Context, StatusRequest) (ConnectorStatus, error)
}

type AppActionHandler interface {
	CapabilityAppAction(context.Context, AppActionRequest) (AppActionResult, error)
}

type healthSnapshotProvider interface {
	ConnectorHealthSnapshot() model.ConnectorHealthSnapshot
}

type connectorAdapters struct {
	presence    PresenceAdapter
	inbox       InboxAdapter
	inboxUpdate InboxUpdater
	directory   DirectoryAdapter
	resolver    TargetResolver
	sender      Sender
	status      StatusAdapter
}

type Registry struct {
	mu         sync.RWMutex
	connectors map[string]connectorAdapters
	aliases    map[string]string
	appActions map[string]AppActionHandler
}

func NewRegistry() *Registry {
	return &Registry{
		connectors: make(map[string]connectorAdapters),
		aliases:    make(map[string]string),
		appActions: make(map[string]AppActionHandler),
	}
}

func (r *Registry) RegisterConnector(conn model.Connector) {
	if r == nil || conn == nil {
		return
	}
	meta := model.NormalizeConnectorMetadata(conn.Metadata())
	if meta.ID == "" {
		return
	}

	adapters := connectorAdapters{}
	if adapter, ok := conn.(PresenceAdapter); ok {
		adapters.presence = adapter
	}
	if adapter, ok := conn.(InboxAdapter); ok {
		adapters.inbox = adapter
	}
	if adapter, ok := conn.(InboxUpdater); ok {
		adapters.inboxUpdate = adapter
	}
	if adapter, ok := conn.(DirectoryAdapter); ok {
		adapters.directory = adapter
	}
	if adapter, ok := conn.(TargetResolver); ok {
		adapters.resolver = adapter
	}
	if adapter, ok := conn.(Sender); ok {
		adapters.sender = adapter
	}
	if adapter, ok := conn.(StatusAdapter); ok {
		adapters.status = adapter
	} else if provider, ok := conn.(healthSnapshotProvider); ok {
		adapters.status = healthStatusAdapter{provider: provider}
	}
	if adapters == (connectorAdapters{}) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectors[meta.ID] = adapters
	for _, alias := range meta.Aliases {
		if alias != "" {
			r.aliases[alias] = meta.ID
		}
	}
}

func (r *Registry) RegisterAppAction(name string, handler AppActionHandler) {
	if r == nil || handler == nil {
		return
	}
	actionName := strings.TrimSpace(name)
	if actionName == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.appActions[actionName] = handler
}

func (r *Registry) PresencePolicy(ctx context.Context, connectorID string) (PresencePolicy, error) {
	adapter, normalizedID, err := r.lookupConnector(connectorID)
	if err != nil {
		return PresencePolicy{}, err
	}
	if adapter.presence == nil {
		return PresencePolicy{}, fmt.Errorf("capabilities: connector %q does not support presence", normalizedID)
	}
	return adapter.presence.CapabilityPresencePolicy(ctx), nil
}

func (r *Registry) EmitPresence(ctx context.Context, req PresenceEmitRequest) error {
	adapter, connectorID, err := r.lookupConnector(req.ConnectorID)
	if err != nil {
		return err
	}
	if adapter.presence == nil {
		return fmt.Errorf("capabilities: connector %q does not support presence", connectorID)
	}
	req.ConnectorID = connectorID
	req.ThreadID = strings.TrimSpace(req.ThreadID)
	req.ThreadType = strings.TrimSpace(req.ThreadType)
	if req.ThreadID == "" {
		return fmt.Errorf("capabilities: thread_id is required")
	}
	if req.Mode == "" {
		req.Mode = PresenceModeTyping
	}
	return adapter.presence.CapabilityEmitPresence(ctx, req)
}

func (r *Registry) DirectoryList(ctx context.Context, req DirectoryListRequest) (DirectoryListResult, error) {
	adapter, connectorID, err := r.lookupConnector(req.ConnectorID)
	if err != nil {
		return DirectoryListResult{}, err
	}
	if adapter.directory == nil {
		return DirectoryListResult{}, fmt.Errorf("capabilities: connector %q does not support directory listing", connectorID)
	}
	req.ConnectorID = connectorID
	if req.Limit <= 0 {
		req.Limit = 50
	}
	return adapter.directory.CapabilityListDirectory(ctx, req)
}

func (r *Registry) InboxList(ctx context.Context, req InboxListRequest) (InboxListResult, error) {
	adapter, connectorID, err := r.lookupConnector(req.ConnectorID)
	if err != nil {
		return InboxListResult{}, err
	}
	if adapter.inbox == nil {
		return InboxListResult{}, fmt.Errorf("capabilities: connector %q does not support inbox listing", connectorID)
	}
	req.ConnectorID = connectorID
	if req.Limit <= 0 {
		req.Limit = 50
	}
	return adapter.inbox.CapabilityListInbox(ctx, req)
}

func (r *Registry) InboxUpdate(ctx context.Context, req InboxUpdateRequest) (InboxUpdateResult, error) {
	adapter, connectorID, err := r.lookupConnector(req.ConnectorID)
	if err != nil {
		return InboxUpdateResult{}, err
	}
	if adapter.inboxUpdate == nil {
		return InboxUpdateResult{}, fmt.Errorf("capabilities: connector %q does not support inbox updates", connectorID)
	}
	req.ConnectorID = connectorID
	return adapter.inboxUpdate.CapabilityUpdateInbox(ctx, req)
}

func (r *Registry) ResolveTarget(ctx context.Context, req TargetResolveRequest) (TargetResolveResult, error) {
	adapter, connectorID, err := r.lookupConnector(req.ConnectorID)
	if err != nil {
		return TargetResolveResult{}, err
	}
	if adapter.resolver == nil {
		return TargetResolveResult{}, fmt.Errorf("capabilities: connector %q does not support target resolution", connectorID)
	}
	req.ConnectorID = connectorID
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return TargetResolveResult{}, fmt.Errorf("capabilities: query is required")
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	return adapter.resolver.CapabilityResolveTarget(ctx, req)
}

func (r *Registry) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	adapter, connectorID, err := r.lookupConnector(req.ConnectorID)
	if err != nil {
		return SendResult{}, err
	}
	if adapter.sender == nil {
		return SendResult{}, fmt.Errorf("capabilities: connector %q does not support direct send", connectorID)
	}
	req.ConnectorID = connectorID
	req.TargetID = strings.TrimSpace(req.TargetID)
	req.TargetType = strings.TrimSpace(req.TargetType)
	req.Message = strings.TrimSpace(req.Message)
	if req.TargetID == "" {
		return SendResult{}, fmt.Errorf("capabilities: target_id is required")
	}
	if req.Message == "" {
		return SendResult{}, fmt.Errorf("capabilities: message is required")
	}
	return adapter.sender.CapabilitySend(ctx, req)
}

func (r *Registry) Status(ctx context.Context, req StatusRequest) (StatusResult, error) {
	if r == nil {
		return StatusResult{}, fmt.Errorf("capabilities: registry is required")
	}
	if connectorID := strings.TrimSpace(req.ConnectorID); connectorID != "" {
		adapter, normalizedID, err := r.lookupConnector(connectorID)
		if err != nil {
			return StatusResult{}, err
		}
		if adapter.status == nil {
			return StatusResult{}, fmt.Errorf("capabilities: connector %q does not support status", normalizedID)
		}
		status, err := adapter.status.CapabilityStatus(ctx, StatusRequest{ConnectorID: normalizedID})
		if err != nil {
			return StatusResult{}, err
		}
		return StatusResult{Connectors: []ConnectorStatus{status}}, nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.connectors))
	for connectorID, adapter := range r.connectors {
		if adapter.status == nil {
			continue
		}
		ids = append(ids, connectorID)
	}
	sort.Strings(ids)

	statuses := make([]ConnectorStatus, 0, len(ids))
	for _, connectorID := range ids {
		status, err := r.connectors[connectorID].status.CapabilityStatus(ctx, StatusRequest{ConnectorID: connectorID})
		if err != nil {
			return StatusResult{}, err
		}
		statuses = append(statuses, status)
	}
	return StatusResult{Connectors: statuses}, nil
}

func (r *Registry) AppAction(ctx context.Context, req AppActionRequest) (AppActionResult, error) {
	if r == nil {
		return AppActionResult{}, fmt.Errorf("capabilities: registry is required")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return AppActionResult{}, fmt.Errorf("capabilities: name is required")
	}

	r.mu.RLock()
	handler, ok := r.appActions[req.Name]
	r.mu.RUnlock()
	if !ok {
		return AppActionResult{}, fmt.Errorf("capabilities: app action %q is not registered", req.Name)
	}
	return handler.CapabilityAppAction(ctx, req)
}

func (r *Registry) lookupConnector(connectorID string) (connectorAdapters, string, error) {
	if r == nil {
		return connectorAdapters{}, "", fmt.Errorf("capabilities: registry is required")
	}
	normalizedID := strings.TrimSpace(connectorID)
	if normalizedID == "" {
		return connectorAdapters{}, "", fmt.Errorf("capabilities: connector_id is required")
	}
	normalizedID = strings.ToLower(normalizedID)

	r.mu.RLock()
	defer r.mu.RUnlock()
	if canonicalID, ok := r.aliases[normalizedID]; ok {
		normalizedID = canonicalID
	}
	adapters, ok := r.connectors[normalizedID]
	if !ok {
		return connectorAdapters{}, "", fmt.Errorf("capabilities: connector %q is not registered", normalizedID)
	}
	return adapters, normalizedID, nil
}

type healthStatusAdapter struct {
	provider healthSnapshotProvider
}

func (h healthStatusAdapter) CapabilityStatus(_ context.Context, _ StatusRequest) (ConnectorStatus, error) {
	snapshot := h.provider.ConnectorHealthSnapshot()
	return ConnectorStatus{
		ConnectorID:      strings.TrimSpace(snapshot.ConnectorID),
		State:            snapshot.State,
		Summary:          snapshot.Summary,
		CheckedAt:        snapshot.CheckedAt,
		RestartSuggested: snapshot.RestartSuggested,
	}, nil
}
