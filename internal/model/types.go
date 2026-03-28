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
	EventID     string
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

type SessionMessageProvenanceKind string

const (
	MessageProvenanceInbound       SessionMessageProvenanceKind = "inbound"
	MessageProvenanceAssistantTurn SessionMessageProvenanceKind = "assistant_turn"
	MessageProvenanceInterSession  SessionMessageProvenanceKind = "inter_session"
)

type RunPhase string

const (
	PhaseReasoning    RunPhase = "reasoning"
	PhaseVerification RunPhase = "verification"
	PhaseSynthesis    RunPhase = "synthesis"
	PhaseEscalation   RunPhase = "escalation"
)

type BaseProfile string

const (
	BaseProfileOperator BaseProfile = "operator"
	BaseProfileResearch BaseProfile = "research"
	BaseProfileWrite    BaseProfile = "write"
	BaseProfileReview   BaseProfile = "review"
	BaseProfileVerify   BaseProfile = "verify"
)

var validBaseProfiles = map[BaseProfile]bool{
	BaseProfileOperator: true,
	BaseProfileResearch: true,
	BaseProfileWrite:    true,
	BaseProfileReview:   true,
	BaseProfileVerify:   true,
}

func IsValidBaseProfile(s string) bool {
	return validBaseProfiles[BaseProfile(s)]
}

type ToolFamily string

const (
	ToolFamilyRepoRead            ToolFamily = "repo_read"
	ToolFamilyRepoWrite           ToolFamily = "repo_write"
	ToolFamilyRuntimeCapability   ToolFamily = "runtime_capability"
	ToolFamilyConnectorCapability ToolFamily = "connector_capability"
	ToolFamilyWebRead             ToolFamily = "web_read"
	ToolFamilyDelegate            ToolFamily = "delegate"
	ToolFamilyVerification        ToolFamily = "verification"
	ToolFamilyDiffReview          ToolFamily = "diff_review"
)

var validToolFamilies = map[ToolFamily]bool{
	ToolFamilyRepoRead:            true,
	ToolFamilyRepoWrite:           true,
	ToolFamilyRuntimeCapability:   true,
	ToolFamilyConnectorCapability: true,
	ToolFamilyWebRead:             true,
	ToolFamilyDelegate:            true,
	ToolFamilyVerification:        true,
	ToolFamilyDiffReview:          true,
}

func IsValidToolFamily(s string) bool {
	return validToolFamilies[ToolFamily(s)]
}

type DelegationKind string

const (
	DelegationKindResearch DelegationKind = "research"
	DelegationKindWrite    DelegationKind = "write"
	DelegationKindReview   DelegationKind = "review"
	DelegationKindVerify   DelegationKind = "verify"
)

var validDelegationKinds = map[DelegationKind]bool{
	DelegationKindResearch: true,
	DelegationKindWrite:    true,
	DelegationKindReview:   true,
	DelegationKindVerify:   true,
}

func IsValidDelegationKind(s string) bool {
	return validDelegationKinds[DelegationKind(s)]
}

type SpecialistSummaryVisibility string

const (
	SpecialistSummaryNone  SpecialistSummaryVisibility = "none"
	SpecialistSummaryBasic SpecialistSummaryVisibility = "basic"
	SpecialistSummaryFull  SpecialistSummaryVisibility = "full"
)

var validSpecialistSummaryVisibility = map[SpecialistSummaryVisibility]bool{
	SpecialistSummaryNone:  true,
	SpecialistSummaryBasic: true,
	SpecialistSummaryFull:  true,
}

func IsValidSpecialistSummaryVisibility(s string) bool {
	return validSpecialistSummaryVisibility[SpecialistSummaryVisibility(s)]
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
	Provenance      SessionMessageProvenance
	CreatedAt       time.Time
}

type SessionMessageProvenance struct {
	Kind              SessionMessageProvenanceKind `json:"kind,omitempty"`
	SourceSessionID   string                       `json:"source_session_id,omitempty"`
	SourceRunID       string                       `json:"source_run_id,omitempty"`
	SourceConnectorID string                       `json:"source_connector_id,omitempty"`
	SourceThreadID    string                       `json:"source_thread_id,omitempty"`
	SourceMessageID   string                       `json:"source_message_id,omitempty"`
	LanguageHint      string                       `json:"language_hint,omitempty"`
	SourceTool        string                       `json:"source_tool,omitempty"`
}

type SessionRoute struct {
	ID                 string
	SessionID          string
	ThreadID           string
	ConnectorID        string
	AccountID          string
	ExternalID         string
	Status             string
	CreatedAt          time.Time
	DeactivatedAt      *time.Time
	DeactivationReason string
	ReplacedByRouteID  string
}

type OutboundIntent struct {
	ID            string
	RunID         string
	ConnectorID   string
	ChatID        string
	MessageText   string
	MetadataJSON  []byte
	DedupeKey     string
	Status        string
	Attempts      int
	CreatedAt     time.Time
	LastAttemptAt *time.Time
}

type OutboundActionButton struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type OutboundIntentMetadata struct {
	ActionButtons []OutboundActionButton `json:"action_buttons,omitempty"`
}

type DeliveryFailure struct {
	ID          string
	IntentID    string
	RunID       string
	ConnectorID string
	ChatID      string
	EventKind   string
	Error       string
	CreatedAt   time.Time
}

type ConnectorDeliveryHealth struct {
	ConnectorID      string
	PendingCount     int
	RetryingCount    int
	TerminalCount    int
	OldestPendingAt  *time.Time
	OldestRetryingAt *time.Time
}

type ConnectorHealthState string

const (
	ConnectorHealthUnknown  ConnectorHealthState = "unknown"
	ConnectorHealthHealthy  ConnectorHealthState = "healthy"
	ConnectorHealthDegraded ConnectorHealthState = "degraded"
)

type ConnectorHealthSnapshot struct {
	ConnectorID      string
	State            ConnectorHealthState
	Summary          string
	CheckedAt        time.Time
	RestartSuggested bool
}

type DeliveryQueueItem struct {
	OutboundIntent
	SessionID      string
	ConversationID string
}

type RouteDirectoryItem struct {
	SessionRoute
	ConversationID string
	AgentID        string
	Role           SessionRole
}

type RunRef struct {
	ID     string
	Status RunStatus
}

type Project struct {
	ID          string
	Name        string
	PrimaryPath string
	RootsJSON   string
	PolicyJSON  string
	Source      string
	CreatedAt   time.Time
	LastUsedAt  time.Time
}

type Run struct {
	ID                    string
	ConversationID        string
	AgentID               string
	SessionID             string
	TeamID                string
	ProjectID             string
	ParentRunID           string
	Objective             string
	CWD                   string
	AuthorityJSON         []byte
	Status                RunStatus
	ExecutionSnapshotJSON []byte
	InputTokens           int
	OutputTokens          int
	ModelLane             string
	ModelID               string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type UsageRecord struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	ModelLane    string
	ModelID      string
}

type RunProfile struct {
	RunID        string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	AccountID    string
}

type AgentProfile struct {
	AgentID                     string
	Role                        string
	Instructions                string
	BaseProfile                 BaseProfile
	ToolFamilies                []ToolFamily
	AllowTools                  []string
	DenyTools                   []string
	DelegationKinds             []DelegationKind
	SpecialistSummaryVisibility SpecialistSummaryVisibility
	MemoryScope                 string
	CanMessage                  []string
}

type ExecutionSnapshot struct {
	TeamID       string                  `json:"team_id"`
	FrontAgentID string                  `json:"front_agent_id"`
	Agents       map[string]AgentProfile `json:"agents"`
}

type ToolSpec struct {
	Name            string
	Description     string
	InputSchemaJSON string
	Family          ToolFamily
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
	ProjectID  string
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
	ProjectID string
	AgentID   string
	Scope     string
	Keyword   string
	Limit     int
}

type MemoryCandidate struct {
	ProjectID      string
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
	ProjectID  string
	RunID      string
	Content    string
	TokenCount int
}

type Conversation struct {
	ID          string
	Key         string
	ConnectorID string
	AccountID   string
	ExternalID  string
	ThreadID    string
	ProjectID   string
	CreatedAt   time.Time
}

type RunReceipt struct {
	ID                 string
	RunID              string
	InputTokens        int
	OutputTokens       int
	CostUSD            float64
	ModelLane          string
	ModelID            string
	VerificationStatus string
	ApprovalCount      int
	BudgetStatus       string
	WallClockMs        int64
	CreatedAt          time.Time
}

type ApprovalRequest struct {
	RunID       string
	ToolName    string
	ArgsJSON    []byte
	BindingJSON []byte
}

type ApprovalTicket struct {
	ID          string
	RunID       string
	ToolName    string
	ArgsJSON    []byte
	BindingJSON []byte
	Fingerprint string
	Status      string
	CreatedAt   time.Time
}

type ConversationGateKind string

const (
	ConversationGateApproval ConversationGateKind = "approval"
)

type ConversationGateStatus string

const (
	ConversationGatePending  ConversationGateStatus = "pending"
	ConversationGateQueued   ConversationGateStatus = "queued"
	ConversationGateResolved ConversationGateStatus = "resolved"
	ConversationGateExpired  ConversationGateStatus = "expired"
)

type ConversationGate struct {
	ID             string
	ConversationID string
	RunID          string
	SessionID      string
	Kind           ConversationGateKind
	Status         ConversationGateStatus
	ApprovalID     string
	Title          string
	Body           string
	OptionsJSON    []byte
	MetadataJSON   []byte
	LanguageHint   string
	CreatedAt      time.Time
	ResolvedAt     *time.Time
}
