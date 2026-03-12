package hitl

// HITLDecision is the resolved decision for a PermissionRequest.
// Allow=false, Always=false, Stop=false is the safe zero-value (deny once).
type HITLDecision struct {
	Allow  bool
	Always bool
	// Stop is true when the user pressed "⏹ Stop" (deny + abort the active agent session).
	// The caller (opencode.Service or claudecode.Service) checks this field and calls
	// its own Stop(ctx) method if true. This avoids a circular dependency between
	// hitl.Service and the agent services.
	Stop bool
}

// Option is a single selectable answer for a Question.
type Option struct {
	Label       string
	Description string
}

// Question is a single question within a QuestionRequest.
type Question struct {
	Question string
	Header   string
	Options  []Option
	Multiple bool // if true, multiple options may be selected (v1: not used by OpenCode)
	Custom   bool // if true, show "Type your own" button for free-text reply
}

// PermissionRequest is sent by opencode.Service or claudecode.Service when an agent
// requests approval for a tool use (e.g. edit, run).
//
// Lifecycle:
//  1. Caller creates a buffered chan of size 1: decisionCh := make(chan HITLDecision, 1)
//  2. Caller calls hitl.Approver.RequestPermission — non-blocking, returns immediately.
//  3. Caller blocks on <-decisionCh (with a select + time.After for HITLTimeout).
//  4. hitl.Service sends exactly one HITLDecision on the channel when resolved.
//
// DecisionCh is write-only from hitl.Service's perspective (chan<- HITLDecision).
// The channel must be buffered (size ≥ 1) to prevent hitl.Service from blocking if
// the caller's select has already timed out.
type PermissionRequest struct {
	ChatID     int64
	ID         string // "permission_<ulid>", must be globally unique
	SessionID  string
	Permission string   // "edit", "run", "bash", etc.
	Patterns   []string // file paths or glob patterns the tool targets
	DecisionCh chan<- HITLDecision
}

// QuestionRequest is sent by opencode.Service when an SSE question.asked event fires.
// Questions are answered sequentially; the caller blocks until all are answered or timed out.
// QuestionRequests are NOT written to hitl_pending (design §9.17).
type QuestionRequest struct {
	ChatID    int64
	ID        string // "question_<ulid>"
	SessionID string
	Questions []Question
}
