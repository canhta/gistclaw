package model

import (
	"context"
	"time"
)

type ProviderErrorCode string

const (
	ErrRateLimit             ProviderErrorCode = "rate_limit"
	ErrContextWindowExceeded ProviderErrorCode = "context_window_exceeded"
	ErrModelRefusal          ProviderErrorCode = "model_refusal"
	ErrProviderTimeout       ProviderErrorCode = "provider_timeout"
	ErrMalformedResponse     ProviderErrorCode = "malformed_response"
)

type ProviderError struct {
	Code      ProviderErrorCode
	Message   string
	Retryable bool
}

func (e *ProviderError) Error() string {
	return string(e.Code) + ": " + e.Message
}

type AttachmentRef struct {
	Name string
	Path string
}

type CapabilitySet struct {
	CanReply bool
	CanEdit  bool
}

type Envelope struct {
	ConnectorID, AccountID, ActorID, ConversationID, ThreadID, MessageID string
	Text                                                                 string
	Attachments                                                          []AttachmentRef
	ReceivedAt                                                           time.Time
	Capabilities                                                         CapabilitySet
	Metadata                                                             map[string]string
}

type ReplayDelta struct {
	RunID       string
	Kind        string
	PayloadJSON []byte
	OccurredAt  time.Time
}

type RunEventSink interface {
	Emit(ctx context.Context, runID string, evt ReplayDelta) error
}

type NoopEventSink struct{}

func (n *NoopEventSink) Emit(_ context.Context, _ string, _ ReplayDelta) error {
	return nil
}

type RunStatus string

const (
	RunStatusPending       RunStatus = "pending"
	RunStatusActive        RunStatus = "active"
	RunStatusNeedsApproval RunStatus = "needs_approval"
	RunStatusCompleted     RunStatus = "completed"
	RunStatusInterrupted   RunStatus = "interrupted"
	RunStatusFailed        RunStatus = "failed"
)

type SessionRole string

const (
	SessionRoleFront  SessionRole = "front"
	SessionRoleWorker SessionRole = "worker"
)

type SessionMessageKind string

const (
	MessageUser      SessionMessageKind = "user"
	MessageAssistant SessionMessageKind = "assistant"
	MessageSpawn     SessionMessageKind = "spawn"
	MessageAnnounce  SessionMessageKind = "announce"
	MessageSteer     SessionMessageKind = "steer"
	MessageAgentSend SessionMessageKind = "agent_send"
)

type RunPhase string

const (
	PhaseReasoning    RunPhase = "reasoning"
	PhaseVerification RunPhase = "verification"
	PhaseSynthesis    RunPhase = "synthesis"
	PhaseEscalation   RunPhase = "escalation"
)

type AgentCapability string

const (
	CapWorkspaceWrite AgentCapability = "workspace_write"
	CapOperatorFacing AgentCapability = "operator_facing"
	CapReadHeavy      AgentCapability = "read_heavy"
	CapProposeOnly    AgentCapability = "propose_only"
)

// validCapabilities is the canonical set of allowed agent capability flag strings.
var validCapabilities = map[AgentCapability]bool{
	CapWorkspaceWrite: true,
	CapOperatorFacing: true,
	CapReadHeavy:      true,
	CapProposeOnly:    true,
}

// IsValidCapability reports whether s names a known AgentCapability flag.
func IsValidCapability(s string) bool {
	return validCapabilities[AgentCapability(s)]
}

type ToolRisk string

const (
	RiskLow    ToolRisk = "low"
	RiskMedium ToolRisk = "medium"
	RiskHigh   ToolRisk = "high"
)

type DecisionMode string

const (
	DecisionAllow DecisionMode = "allow"
	DecisionAsk   DecisionMode = "ask"
	DecisionDeny  DecisionMode = "deny"
)

type Event struct {
	ID             string
	ConversationID string
	RunID          string
	ParentRunID    string
	Kind           string
	PayloadJSON    []byte
	CreatedAt      time.Time
}

type Session struct {
	ID                  string
	ConversationID      string
	Key                 string
	AgentID             string
	Role                SessionRole
	ParentSessionID     string
	ControllerSessionID string
	Status              string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type SessionMessage struct {
	ID              string
	SessionID       string
	SenderSessionID string
	Kind            SessionMessageKind
	Body            string
	CreatedAt       time.Time
}

type RunRef struct {
	ID     string
	Status RunStatus
}

type Run struct {
	ID                    string
	ConversationID        string
	AgentID               string
	TeamID                string
	ParentRunID           string
	Objective             string
	WorkspaceRoot         string
	Status                RunStatus
	ExecutionSnapshotJSON []byte
	InputTokens           int
	OutputTokens          int
	ModelLane             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type UsageRecord struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	ModelLane    string
}

type RunProfile struct {
	RunID        string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	AccountID    string
}

type AgentProfile struct {
	AgentID      string
	Capabilities []AgentCapability
	ToolProfile  string
	MemoryScope  string
}

type ToolSpec struct {
	Name            string
	Description     string
	InputSchemaJSON string
	Risk            ToolRisk
	SideEffect      string
	Approval        string
}

type ToolCall struct {
	ID        string
	ToolName  string
	InputJSON []byte
}

type ToolCallRequest struct {
	ID        string
	ToolName  string
	InputJSON []byte
}

type ToolResult struct {
	Output string
	Error  string
}

type ToolDecision struct {
	Mode   DecisionMode
	Reason string
}

type FileChange struct {
	Path    string
	Content []byte
	Op      string
}

type ChangePreview struct {
	RunID   string
	Changes []FileChange
	Diff    string
}

type ApplyResult struct {
	Applied bool
	Error   string
}

type MemoryItem struct {
	ID         string
	AgentID    string
	Scope      string
	Content    string
	Source     string
	Provenance string
	Confidence float64
	DedupeKey  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type MemoryQuery struct {
	AgentID string
	Scope   string
	Keyword string
	Limit   int
}

type MemoryCandidate struct {
	AgentID        string
	Scope          string
	Content        string
	Provenance     string
	Confidence     float64
	DedupeKey      string
	ConversationID string
}

type SummaryRef struct {
	ID         string
	RunID      string
	Content    string
	TokenCount int
}

type Conversation struct {
	ID        string
	Key       string
	CreatedAt time.Time
}

type RunReceipt struct {
	ID                 string
	RunID              string
	InputTokens        int
	OutputTokens       int
	CostUSD            float64
	ModelLane          string
	VerificationStatus string
	ApprovalCount      int
	BudgetStatus       string
	WallClockMs        int64
	CreatedAt          time.Time
}

type ApprovalRequest struct {
	RunID      string
	ToolName   string
	ArgsJSON   []byte
	TargetPath string
}

type ApprovalTicket struct {
	ID          string
	RunID       string
	ToolName    string
	ArgsJSON    []byte
	TargetPath  string
	Fingerprint string
	Status      string
	CreatedAt   time.Time
}
