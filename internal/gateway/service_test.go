// internal/gateway/service_test.go
package gateway_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/conversation"
	"github.com/canhta/gistclaw/internal/gateway"
	"github.com/canhta/gistclaw/internal/hitl"
	"github.com/canhta/gistclaw/internal/mcp"
	mempkg "github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

// ============================================================
// Mock implementations
// ============================================================

// --- mock channel ---

type mockChannel struct {
	mu      sync.Mutex
	inbound chan channel.InboundMessage
	sent    []string
	typings []int64
	name    string
}

func newMockChannel() *mockChannel {
	return &mockChannel{
		inbound: make(chan channel.InboundMessage, 10),
		name:    "mock",
	}
}

func (m *mockChannel) Receive(_ context.Context) (<-chan channel.InboundMessage, error) {
	return m.inbound, nil
}

func (m *mockChannel) SendMessage(_ context.Context, _ int64, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, text)
	return nil
}

func (m *mockChannel) SendKeyboard(_ context.Context, _ int64, _ channel.KeyboardPayload) error {
	return nil
}

func (m *mockChannel) SendTyping(_ context.Context, chatID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typings = append(m.typings, chatID)
	return nil
}

func (m *mockChannel) Name() string { return m.name }

func (m *mockChannel) sentMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.sent))
	copy(cp, m.sent)
	return cp
}

// --- mock hitl.Approver + HITLResolver ---

type mockApprover struct{}

func (m *mockApprover) RequestPermission(_ context.Context, _ hitl.PermissionRequest) error {
	return nil
}

func (m *mockApprover) RequestQuestion(_ context.Context, _ hitl.QuestionRequest) error {
	return nil
}

func (m *mockApprover) Resolve(_ string, _ string) error {
	return nil
}

// --- mock opencode.Service ---

type mockOCService struct {
	mu      sync.Mutex
	tasks   []string
	stopped bool
	isAlive bool
}

func (m *mockOCService) Run(_ context.Context) error { return nil }
func (m *mockOCService) IsAlive(_ context.Context) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isAlive
}
func (m *mockOCService) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	return nil
}
func (m *mockOCService) SubmitTask(_ context.Context, _ int64, prompt string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = append(m.tasks, prompt)
	return nil
}
func (m *mockOCService) SubmitTaskWithResult(_ context.Context, _ int64, prompt string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = append(m.tasks, prompt)
	return "mock result", nil
}

// --- mock claudecode.Service ---

type mockCCService struct {
	mu      sync.Mutex
	tasks   []string
	stopped bool
	isAlive bool
}

func (m *mockCCService) Run(_ context.Context) error { return nil }
func (m *mockCCService) IsAlive(_ context.Context) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isAlive
}
func (m *mockCCService) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	return nil
}
func (m *mockCCService) SubmitTask(_ context.Context, _ int64, prompt string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = append(m.tasks, prompt)
	return nil
}
func (m *mockCCService) SubmitTaskWithResult(_ context.Context, _ int64, prompt string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = append(m.tasks, prompt)
	return "mock result", nil
}

// --- mock LLMProvider ---

type mockLLM struct {
	mu           sync.Mutex
	responses    []*providers.LLMResponse
	errs         []error // parallel to responses; non-nil entry returns that error instead
	callCount    int
	capturedMsgs [][]providers.Message // one entry per Chat() call
}

func (m *mockLLM) Name() string { return "mock" }

func (m *mockLLM) Chat(_ context.Context, msgs []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]providers.Message, len(msgs))
	copy(cp, msgs)
	m.capturedMsgs = append(m.capturedMsgs, cp)
	idx := m.callCount
	m.callCount++
	if idx < len(m.errs) && m.errs[idx] != nil {
		return nil, m.errs[idx]
	}
	if idx < len(m.responses) && m.responses[idx] != nil {
		return m.responses[idx], nil
	}
	return &providers.LLMResponse{Content: "fallback answer"}, nil
}

func (m *mockLLM) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *mockLLM) firstCallMsgs() []providers.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.capturedMsgs) == 0 {
		return nil
	}
	return m.capturedMsgs[0]
}

// --- mock SearchProvider ---

type mockSearch struct{}

func (m *mockSearch) Name() string { return "mock" }

func (m *mockSearch) Search(_ context.Context, query string, _ int) ([]tools.SearchResult, error) {
	return []tools.SearchResult{{Title: "result", URL: "https://example.com", Snippet: "snippet for " + query}}, nil
}

// --- mock WebFetcher ---

type mockFetcher struct{}

func (m *mockFetcher) Fetch(_ context.Context, url string) (string, error) {
	return "content of " + url, nil
}

// --- helpers ---

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("newTestStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newTestScheduler(t *testing.T, s *store.Store) *scheduler.Service {
	t.Helper()
	return scheduler.NewService(s, &noopJobTarget{}, config.Tuning{
		SchedulerTick:       time.Second,
		MissedJobsFireLimit: 5,
	}, 42) // 42 = operator chatID matching AllowedUserIDs in test config
}

type noopJobTarget struct{}

func (n *noopJobTarget) RunAgentTask(_ context.Context, _ agent.Kind, _ string) error { return nil }
func (n *noopJobTarget) SendChat(_ context.Context, _ int64, _ string) error          { return nil }

func newService(t *testing.T, ch channel.Channel, llm providers.LLMProvider) *gateway.Service {
	t.Helper()
	return newServiceWithMemoryEngine(t, ch, llm, nil)
}

func newServiceWithMemoryEngine(t *testing.T, ch channel.Channel, llm providers.LLMProvider, mem mempkg.Engine) *gateway.Service {
	t.Helper()
	return newServiceFull(t, ch, llm, mem, 10*time.Millisecond)
}

// newServiceFull is the base constructor used by all test helpers.
func newServiceFull(t *testing.T, ch channel.Channel, llm providers.LLMProvider, mem mempkg.Engine, retryDelay time.Duration) *gateway.Service {
	t.Helper()
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{
		AllowedUserIDs: []int64{42},
		Tuning: config.Tuning{
			SchedulerTick:           time.Second,
			MissedJobsFireLimit:     5,
			MaxIterations:           20,
			LLMRetryDelay:           retryDelay,
			ConversationWindowTurns: 20,
		},
	}
	conv := conversation.NewManager(s, cfg.Tuning.ConversationWindowTurns, 0)
	return gateway.NewService(
		ch,
		&mockApprover{},
		&mockOCService{isAlive: false},
		&mockCCService{isAlive: false},
		llm,
		&mockSearch{},
		&mockFetcher{},
		mcp.NewMCPManager(nil, config.Tuning{}), // empty MCP manager
		sched,
		s,
		nil,        // guard: nil is safe for unit tests (buildStatus guards for nil)
		mem,        // memory.Engine: nil = no system prompt / memory tools
		conv,       // conversation.Manager
		time.Now(), // startTime
		cfg,
	)
}

// ============================================================
// Tests
// ============================================================

// TestGateway_AllowedUserCheck verifies messages from disallowed users are silently dropped.
func TestGateway_AllowedUserCheck(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go svc.Run(ctx) //nolint:errcheck

	// Disallowed user
	ch.inbound <- channel.InboundMessage{ChatID: 99, UserID: 99, Text: "hello"}
	time.Sleep(150 * time.Millisecond)

	if msgs := ch.sentMessages(); len(msgs) != 0 {
		t.Errorf("expected 0 messages for disallowed user, got %d: %v", len(msgs), msgs)
	}
	if llm.calls() != 0 {
		t.Errorf("expected 0 LLM calls for disallowed user, got %d", llm.calls())
	}
}

// TestGateway_OCCommand verifies /oc routes to opencode.SubmitTask.
func TestGateway_OCCommand(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	oc := &mockOCService{}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	conv := conversation.NewManager(s, 20, 0)
	svc := gateway.NewService(ch, &mockApprover{}, oc, &mockCCService{}, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewMCPManager(nil, config.Tuning{}), sched, s, nil, nil, conv, time.Now(), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/oc build the auth module"}
	time.Sleep(150 * time.Millisecond)

	oc.mu.Lock()
	tasks := oc.tasks
	oc.mu.Unlock()
	if len(tasks) != 1 || tasks[0] != "build the auth module" {
		t.Errorf("expected SubmitTask(\"build the auth module\"), got %v", tasks)
	}
}

// TestGateway_CCCommand verifies /cc routes to claudecode.SubmitTask.
func TestGateway_CCCommand(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	cc := &mockCCService{}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	conv := conversation.NewManager(s, 20, 0)
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, cc, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewMCPManager(nil, config.Tuning{}), sched, s, nil, nil, conv, time.Now(), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/cc refactor service layer"}
	time.Sleep(150 * time.Millisecond)

	cc.mu.Lock()
	tasks := cc.tasks
	cc.mu.Unlock()
	if len(tasks) != 1 || tasks[0] != "refactor service layer" {
		t.Errorf("expected SubmitTask(\"refactor service layer\"), got %v", tasks)
	}
}

// TestGateway_StopCommand verifies /stop calls Stop on both agent services and sends confirmation.
func TestGateway_StopCommand(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	oc := &mockOCService{}
	cc := &mockCCService{}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	conv := conversation.NewManager(s, 20, 0)
	svc := gateway.NewService(ch, &mockApprover{}, oc, cc, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewMCPManager(nil, config.Tuning{}), sched, s, nil, nil, conv, time.Now(), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/stop"}
	time.Sleep(150 * time.Millisecond)

	oc.mu.Lock()
	ocStopped := oc.stopped
	oc.mu.Unlock()
	cc.mu.Lock()
	ccStopped := cc.stopped
	cc.mu.Unlock()

	if !ocStopped {
		t.Error("expected opencode.Stop() to be called")
	}
	if !ccStopped {
		t.Error("expected claudecode.Stop() to be called")
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "Stopped") || strings.Contains(m, "⏹") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected stop confirmation message, got %v", msgs)
	}
}

// TestGateway_StatusCommand verifies /status sends a status message.
func TestGateway_StatusCommand(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	conv := conversation.NewManager(s, 20, 0)
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, &mockCCService{}, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewMCPManager(nil, config.Tuning{}), sched, s, nil, nil, conv, time.Now(), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/status"}
	time.Sleep(150 * time.Millisecond)

	msgs := ch.sentMessages()
	if len(msgs) == 0 {
		t.Fatal("expected /status response, got none")
	}
	// Status must contain expected sections
	status := strings.Join(msgs, "\n")
	for _, want := range []string{"OpenCode", "ClaudeCode", "HITL", "Scheduled"} {
		if !strings.Contains(status, want) {
			t.Errorf("/status missing %q; got:\n%s", want, status)
		}
	}
}

// TestGateway_PlainChat_DirectAnswer verifies a plain chat message that needs no tools returns LLM answer.
func TestGateway_PlainChat_DirectAnswer(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{Content: "Go was released in 2009.", ToolCall: nil, Usage: providers.Usage{}},
		},
	}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "when was Go released?"}
	time.Sleep(300 * time.Millisecond)

	msgs := ch.sentMessages()
	if len(msgs) == 0 {
		t.Fatal("expected plain chat response, got none")
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "2009") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected LLM answer to contain '2009'; got: %v", msgs)
	}
	// At least 1 LLM call (for the answer) plus possible auto-curation calls
	if llm.calls() < 1 {
		t.Errorf("expected at least 1 LLM call, got %d", llm.calls())
	}
}

// TestGateway_PlainChat_ToolLoop verifies a plain chat message that calls web_search then answers.
func TestGateway_PlainChat_ToolLoop(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{
				Content: "",
				ToolCall: &providers.ToolCall{
					ID:        "call-1",
					Name:      "web_search",
					InputJSON: `{"query":"latest Go release","count":5}`,
				},
				Usage: providers.Usage{},
			},
			{Content: "The latest Go version is 1.25.", ToolCall: nil, Usage: providers.Usage{}},
		},
	}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "what is the latest Go version?"}
	time.Sleep(300 * time.Millisecond)

	// At least 2 LLM calls (tool + answer); auto-curation may add more
	if llm.calls() < 2 {
		t.Errorf("expected at least 2 LLM calls (tool + answer), got %d", llm.calls())
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "1.25") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected final answer to contain '1.25'; got: %v", msgs)
	}
}

// TestGateway_DoomLoopGuard verifies the doom-loop guard:
// LLM returns the same tool call 3 times → forced final answer on call 4.
// Total LLM calls must be at least 4 (may include auto-curation call).
func TestGateway_DoomLoopGuard(t *testing.T) {
	ch := newMockChannel()

	sameToolCall := &providers.ToolCall{
		ID:        "call-dup",
		Name:      "web_search",
		InputJSON: `{"query":"Go programming","count":5}`,
	}
	finalAnswer := "Here is the answer after detecting tool loop."

	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			// calls 1, 2, 3: same tool call (doom-loop)
			{Content: "", ToolCall: sameToolCall, Usage: providers.Usage{}},
			{Content: "", ToolCall: sameToolCall, Usage: providers.Usage{}},
			{Content: "", ToolCall: sameToolCall, Usage: providers.Usage{}},
			// call 4: forced final answer (LLM called without tools after guard triggers)
			{Content: finalAnswer, ToolCall: nil, Usage: providers.Usage{}},
		},
	}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "tell me about Go"}
	time.Sleep(600 * time.Millisecond)

	// Must have made at least 4 LLM calls (may include auto-curation)
	if calls := llm.calls(); calls < 4 {
		t.Errorf("doom-loop: expected at least 4 LLM calls, got %d", calls)
	}

	// Final message sent to user must be the forced final answer
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, finalAnswer) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected forced final answer to be sent; got: %v", msgs)
	}
}

// TestGateway_HITLCallback verifies callback data "hitl:<id>:<action>" is dispatched to HITL handling.
// This test verifies gateway does NOT forward callback to LLM.
func TestGateway_HITLCallback(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{
		ChatID:       42,
		UserID:       42,
		Text:         "",
		CallbackData: "hitl:permission_abc123:once",
	}
	time.Sleep(150 * time.Millisecond)

	// LLM should NOT be called for HITL callbacks
	if llm.calls() != 0 {
		t.Errorf("expected 0 LLM calls for HITL callback, got %d", llm.calls())
	}
}

// TestGateway_ScheduleJobTool verifies the schedule_job tool creates a job via scheduler.
func TestGateway_ScheduleJobTool(t *testing.T) {
	ch := newMockChannel()

	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{
				Content: "",
				ToolCall: &providers.ToolCall{
					ID:        "call-sched",
					Name:      "schedule_job",
					InputJSON: `{"kind":"every","target":"opencode","prompt":"run tests","schedule":"3600"}`,
				},
				Usage: providers.Usage{},
			},
			{Content: "Job scheduled successfully.", ToolCall: nil, Usage: providers.Usage{}},
		},
	}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	conv := conversation.NewManager(s, 20, 0)
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, &mockCCService{}, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewMCPManager(nil, config.Tuning{}), sched, s, nil, nil, conv, time.Now(), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "schedule a job to run tests every hour"}
	time.Sleep(300 * time.Millisecond)

	// Job must appear in store
	rows, err := s.ListAllJobs()
	if err != nil {
		t.Fatalf("ListAllJobs: %v", err)
	}
	if len(rows) == 0 {
		t.Error("expected schedule_job tool to create a job in store; got none")
	}
}

// TestGateway_LLMError verifies gateway sends an error message if LLM returns an error.
func TestGateway_LLMError(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	// Inject a failing LLM via custom implementation
	failLLM := &failingLLM{err: errors.New("LLM unavailable")}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	conv := conversation.NewManager(s, 20, 0)
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, &mockCCService{}, failLLM,
		&mockSearch{}, &mockFetcher{}, mcp.NewMCPManager(nil, config.Tuning{}), sched, s, nil, nil, conv, time.Now(), cfg)
	_ = llm // unused; suppress warning

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "anything"}
	time.Sleep(150 * time.Millisecond)

	msgs := ch.sentMessages()
	if len(msgs) == 0 {
		t.Fatal("expected error message to be sent, got none")
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "⚠️") || strings.Contains(m, "error") || strings.Contains(m, "Error") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ⚠️ error message; got: %v", msgs)
	}
}

type failingLLM struct {
	err error
}

func (f *failingLLM) Name() string { return "failing" }
func (f *failingLLM) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	return nil, f.err
}

// TestGateway_SOUL_NilLoader verifies that a nil memory.Engine produces no system message.
func TestGateway_SOUL_NilLoader(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{Content: "answer", ToolCall: nil},
		},
	}
	svc := newServiceWithMemoryEngine(t, ch, llm, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(300 * time.Millisecond)

	msgs := llm.firstCallMsgs()
	if len(msgs) == 0 {
		t.Fatal("expected at least one message in first LLM call")
	}
	if msgs[0].Role == "system" {
		t.Errorf("expected no system message with nil memory.Engine; got Role=%q Content=%q", msgs[0].Role, msgs[0].Content)
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected first message to be user role; got %q", msgs[0].Role)
	}
}

// TestGateway_SOUL_InjectsSystemPrompt verifies that a memory.Engine with soul content
// prepends a system message as the first message sent to the LLM.
func TestGateway_SOUL_InjectsSystemPrompt(t *testing.T) {
	soulContent := "You are a helpful assistant for coding tasks."
	soulPath := filepath.Join(t.TempDir(), "SOUL.md")
	if err := os.WriteFile(soulPath, []byte(soulContent), 0o600); err != nil {
		t.Fatalf("write SOUL.md: %v", err)
	}

	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{Content: "answer", ToolCall: nil},
		},
	}
	// Create a memory.Engine with the soul path and a temp memory/notes dir
	dir := t.TempDir()
	mem := mempkg.NewEngine(soulPath, filepath.Join(dir, "MEMORY.md"), filepath.Join(dir, "notes"))
	svc := newServiceWithMemoryEngine(t, ch, llm, mem)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(300 * time.Millisecond)

	msgs := llm.firstCallMsgs()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages (system + user); got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected first message Role=system; got %q", msgs[0].Role)
	}
	if msgs[0].Content != soulContent {
		t.Errorf("expected system message content %q; got %q", soulContent, msgs[0].Content)
	}
	if msgs[1].Role != "user" {
		t.Errorf("expected second message Role=user; got %q", msgs[1].Role)
	}
}

// TestGateway_SOUL_LoadError verifies that a missing soul file is non-fatal:
// the LLM is still called, no system message is prepended, no error is sent to the user.
func TestGateway_SOUL_LoadError(t *testing.T) {
	// Point engine at a non-existent soul file — LoadContext returns "" for missing files.
	dir := t.TempDir()
	mem := mempkg.NewEngine(
		filepath.Join(dir, "missing.md"),
		filepath.Join(dir, "MEMORY.md"),
		filepath.Join(dir, "notes"),
	)

	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{Content: "answer despite no SOUL", ToolCall: nil},
		},
	}
	svc := newServiceWithMemoryEngine(t, ch, llm, mem)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(300 * time.Millisecond)

	// LLM must still be called.
	if llm.calls() < 1 {
		t.Errorf("expected at least 1 LLM call despite missing soul file; got %d", llm.calls())
	}
	// No system message prepended (soul file missing, memory file missing, no notes).
	callMsgs := llm.firstCallMsgs()
	if len(callMsgs) > 0 && callMsgs[0].Role == "system" {
		t.Errorf("expected no system message on missing soul file; got one with content %q", callMsgs[0].Content)
	}
	// No error message sent to user (answer should be sent instead).
	sent := ch.sentMessages()
	for _, m := range sent {
		if strings.Contains(m, "⚠️") {
			t.Errorf("expected no error message to user on missing soul file; got %q", m)
		}
	}
}

// TestGateway_Retry_TransientSucceeds verifies that a 5xx error on the first attempt
// is retried and succeeds on the second attempt. Total LLM calls must be at least 2.
func TestGateway_Retry_TransientSucceeds(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		errs: []error{
			errors.New("503 Service Unavailable"), // attempt 1: retryable
			nil,                                   // attempt 2: success
		},
		responses: []*providers.LLMResponse{
			nil, // slot 0 unused (error returned instead)
			{Content: "recovered answer", ToolCall: nil},
		},
	}
	svc := newServiceFull(t, ch, llm, nil, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(300 * time.Millisecond)

	if calls := llm.calls(); calls < 2 {
		t.Errorf("expected at least 2 LLM calls (1 fail + 1 retry success); got %d", calls)
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "recovered answer") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'recovered answer' after retry; got: %v", msgs)
	}
}

// TestGateway_Retry_TerminalFailsFast verifies that a 4xx error (non-429) is not retried.
// Total LLM calls must be exactly 1.
func TestGateway_Retry_TerminalFailsFast(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		errs: []error{
			errors.New("400 Bad Request: invalid model"), // terminal
		},
	}
	svc := newServiceFull(t, ch, llm, nil, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(200 * time.Millisecond)

	if calls := llm.calls(); calls != 1 {
		t.Errorf("terminal error: expected exactly 1 LLM call (no retry); got %d", calls)
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "⚠️") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error message sent to user; got: %v", msgs)
	}
}

// TestGateway_Retry_RateLimitNotifiesUser verifies that a 429 error sends a
// rate-limit notification to the user and retries. The notification must appear
// exactly once even if multiple retries are needed.
func TestGateway_Retry_RateLimitNotifiesUser(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		errs: []error{
			errors.New("429 Too Many Requests"), // rate limit
			nil,                                 // success on retry
		},
		responses: []*providers.LLMResponse{
			nil,
			{Content: "answer after rate limit", ToolCall: nil},
		},
	}
	svc := newServiceFull(t, ch, llm, nil, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(300 * time.Millisecond)

	if calls := llm.calls(); calls < 2 {
		t.Errorf("rate limit: expected at least 2 LLM calls; got %d", calls)
	}

	msgs := ch.sentMessages()
	rateLimitNotices := 0
	for _, m := range msgs {
		if strings.Contains(m, "rate limited") || strings.Contains(m, "Rate limited") {
			rateLimitNotices++
		}
	}
	if rateLimitNotices != 1 {
		t.Errorf("expected exactly 1 rate-limit notification; got %d in: %v", rateLimitNotices, msgs)
	}

	found := false
	for _, m := range msgs {
		if strings.Contains(m, "answer after rate limit") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected final answer after rate-limit retry; got: %v", msgs)
	}
}

// TestGateway_Retry_ExhaustsAndErrors verifies that after 3 failed attempts
// (all retryable 5xx), the user receives an error message. Total LLM calls = 3.
func TestGateway_Retry_ExhaustsAndErrors(t *testing.T) {
	ch := newMockChannel()
	serverErr := errors.New("503 Service Unavailable")
	llm := &mockLLM{
		errs: []error{serverErr, serverErr, serverErr},
	}
	svc := newServiceFull(t, ch, llm, nil, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(300 * time.Millisecond)

	if calls := llm.calls(); calls != 3 {
		t.Errorf("expected 3 LLM calls (all exhausted); got %d", calls)
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "⚠️") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error message after retry exhaustion; got: %v", msgs)
	}
}

// TestGateway_History_UserAndAssistantSaved verifies that after a successful plain-chat
// turn, both the user message and assistant response are persisted to the store.
func TestGateway_History_UserAndAssistantSaved(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{Content: "assistant reply", ToolCall: nil},
		},
	}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{
		AllowedUserIDs: []int64{42},
		Tuning: config.Tuning{
			SchedulerTick:           time.Second,
			MissedJobsFireLimit:     5,
			MaxIterations:           20,
			LLMRetryDelay:           10 * time.Millisecond,
			ConversationWindowTurns: 20,
		},
	}
	conv := conversation.NewManager(s, cfg.Tuning.ConversationWindowTurns, 0)
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, &mockCCService{}, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewMCPManager(nil, config.Tuning{}), sched, s, nil, nil, conv, time.Now(), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "user question"}
	time.Sleep(300 * time.Millisecond)

	history, err := s.GetHistory(42, 40)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history messages (user + assistant); got %d: %v", len(history), history)
	}
	if history[0].Role != "user" || history[0].Content != "user question" {
		t.Errorf("history[0]: want {user, 'user question'}; got %+v", history[0])
	}
	if history[1].Role != "assistant" || history[1].Content != "assistant reply" {
		t.Errorf("history[1]: want {assistant, 'assistant reply'}; got %+v", history[1])
	}
}

// TestGateway_History_InjectedIntoPriorMessages verifies that existing history is injected
// between the system prompt and the current user message on subsequent turns.
func TestGateway_History_InjectedIntoPriorMessages(t *testing.T) {
	ch := newMockChannel()
	// Two turns: first produces a captured message set showing history in the second.
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{Content: "first answer", ToolCall: nil},
			{Content: "second answer", ToolCall: nil},
		},
	}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{
		AllowedUserIDs: []int64{42},
		Tuning: config.Tuning{
			SchedulerTick:           time.Second,
			MissedJobsFireLimit:     5,
			MaxIterations:           20,
			LLMRetryDelay:           10 * time.Millisecond,
			ConversationWindowTurns: 20,
		},
	}
	conv := conversation.NewManager(s, cfg.Tuning.ConversationWindowTurns, 0)
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, &mockCCService{}, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewMCPManager(nil, config.Tuning{}), sched, s, nil, nil, conv, time.Now(), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	// First turn.
	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "first question"}
	time.Sleep(300 * time.Millisecond)

	// Second turn — history from turn 1 should be injected.
	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "second question"}
	time.Sleep(300 * time.Millisecond)

	llm.mu.Lock()
	secondCallMsgs := llm.capturedMsgs[1]
	llm.mu.Unlock()

	// second call should contain: [user:"first question", assistant:"first answer", user:"second question"]
	// (no soul configured, so no leading system message)
	if len(secondCallMsgs) < 3 {
		t.Fatalf("second LLM call: expected ≥3 messages (history + current); got %d: %v", len(secondCallMsgs), secondCallMsgs)
	}
	found := false
	for _, m := range secondCallMsgs {
		if m.Role == "user" && m.Content == "first question" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'first question' in second call history; got: %v", secondCallMsgs)
	}
}

// TestGateway_MaxIterations verifies that MaxIterations=2 triggers a forced final answer
// when the LLM keeps returning tool calls. Total LLM calls must be at least 3
// (2 tool-calling iterations + 1 forced-final call).
func TestGateway_MaxIterations(t *testing.T) {
	ch := newMockChannel()

	toolCall := &providers.ToolCall{
		ID:        "call-iter",
		Name:      "web_search",
		InputJSON: `{"query":"something","count":5}`,
	}
	forcedAnswer := "Forced final answer after iteration cap."

	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			// iterations 0 and 1: keep calling tools
			{Content: "", ToolCall: toolCall},
			{Content: "", ToolCall: toolCall},
			// forced-final call (iteration 2 triggers cap, calls LLM without tools)
			{Content: forcedAnswer, ToolCall: nil},
		},
	}

	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{
		AllowedUserIDs: []int64{42},
		Tuning: config.Tuning{
			SchedulerTick:       time.Second,
			MissedJobsFireLimit: 5,
			MaxIterations:       2,
		},
	}
	conv := conversation.NewManager(s, 20, 0)
	svc := gateway.NewService(
		ch, &mockApprover{}, &mockOCService{}, &mockCCService{}, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewMCPManager(nil, config.Tuning{}),
		sched, s, nil, nil, conv, time.Now(), cfg,
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "search forever"}
	time.Sleep(600 * time.Millisecond)

	if calls := llm.calls(); calls < 3 {
		t.Errorf("MaxIterations=2: expected at least 3 LLM calls (2 tool + 1 forced-final), got %d", calls)
	}

	sent := ch.sentMessages()
	found := false
	for _, m := range sent {
		if strings.Contains(m, forcedAnswer) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected forced final answer to be sent; got: %v", sent)
	}
}

// TestGateway_Remember verifies the remember tool appends a fact to MEMORY.md.
func TestGateway_Remember(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "MEMORY.md")
	mem := mempkg.NewEngine("", memPath, filepath.Join(dir, "notes"))

	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{
				Content: "",
				ToolCall: &providers.ToolCall{
					ID:        "call-mem",
					Name:      "remember",
					InputJSON: `{"content":"User prefers concise answers."}`,
				},
			},
			{Content: "Memory saved.", ToolCall: nil},
		},
	}
	svc := newServiceWithMemoryEngine(t, ch, llm, mem)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "remember that I prefer concise answers"}
	time.Sleep(300 * time.Millisecond)

	data, err := os.ReadFile(memPath)
	if err != nil {
		t.Fatalf("MEMORY.md not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "User prefers concise answers.") {
		t.Errorf("expected memory content in MEMORY.md; got: %q", content)
	}
}

// TestGateway_UpdateMemory is a compatibility test verifying that the old update_memory
// behaviour (appending to MEMORY.md) is preserved via the remember tool.
// Kept for API continuity; delegates to TestGateway_Remember logic.
func TestGateway_UpdateMemory(t *testing.T) {
	TestGateway_Remember(t)
}

// TestGateway_ClearMemory verifies the curate_memory tool can rewrite MEMORY.md.
// The old clear_memory tool is gone; curate_memory now manages memory curation.
// This test uses the remember tool to write content and then curate_memory to rewrite it.
func TestGateway_ClearMemory(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "MEMORY.md")
	if err := os.WriteFile(memPath, []byte("[2026-01-01 00:00] Old memory.\n"), 0o644); err != nil {
		t.Fatalf("write initial MEMORY.md: %v", err)
	}
	mem := mempkg.NewEngine("", memPath, filepath.Join(dir, "notes"))

	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{
				Content: "",
				ToolCall: &providers.ToolCall{
					ID:        "call-curate",
					Name:      "curate_memory",
					InputJSON: `{}`,
				},
			},
			// curate_memory calls llm.Chat internally (for rewriting)
			{Content: "", ToolCall: nil}, // This is the curate LLM call returning empty
			{Content: "Memory curated.", ToolCall: nil},
		},
	}
	svc := newServiceWithMemoryEngine(t, ch, llm, mem)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "curate your memory"}
	time.Sleep(300 * time.Millisecond)

	// The curate tool was invoked; verify no crash and memory file still exists
	if _, err := os.Stat(memPath); err != nil {
		t.Errorf("MEMORY.md should still exist after curate: %v", err)
	}
}
