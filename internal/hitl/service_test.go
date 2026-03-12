package hitl_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/hitl"
	"github.com/canhta/gistclaw/internal/store"
)

// mockChannel is a fake channel.Channel for tests.
// It captures outbound messages and keyboards, and lets tests inject inbound messages.
type mockChannel struct {
	mu        sync.Mutex
	messages  []string
	keyboards []channel.KeyboardPayload
	inbound   chan channel.InboundMessage
}

func newMockChannel() *mockChannel {
	return &mockChannel{inbound: make(chan channel.InboundMessage, 16)}
}

func (m *mockChannel) Name() string { return "mock" }

func (m *mockChannel) Receive(_ context.Context) (<-chan channel.InboundMessage, error) {
	return m.inbound, nil
}

func (m *mockChannel) SendMessage(_ context.Context, _ int64, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, text)
	return nil
}

func (m *mockChannel) SendKeyboard(_ context.Context, _ int64, payload channel.KeyboardPayload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keyboards = append(m.keyboards, payload)
	return nil
}

func (m *mockChannel) SendTyping(_ context.Context, _ int64) error { return nil }

func (m *mockChannel) lastKeyboard() (channel.KeyboardPayload, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.keyboards) == 0 {
		return channel.KeyboardPayload{}, false
	}
	return m.keyboards[len(m.keyboards)-1], true
}

func (m *mockChannel) inject(msg channel.InboundMessage) {
	m.inbound <- msg
}

// mockReplier captures ReplyQuestion calls for test assertions.
type mockReplier struct {
	mu    sync.Mutex
	calls []replyCall
}

type replyCall struct {
	id      string
	answers [][]string
}

func (m *mockReplier) ReplyQuestion(_ context.Context, id string, answers [][]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, replyCall{id: id, answers: answers})
	return nil
}

func (m *mockReplier) lastCall() (replyCall, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return replyCall{}, false
	}
	return m.calls[len(m.calls)-1], true
}

// newTestService creates a hitl.Service with a mock channel, temp SQLite store, and mock replier.
func newTestService(t *testing.T, tuning config.Tuning) (*hitl.Service, *mockChannel, *store.Store, *mockReplier) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	ch := newMockChannel()
	rep := &mockReplier{}
	svc := hitl.NewService(ch, s, tuning, rep)
	return svc, ch, s, rep
}

func defaultTuning() config.Tuning {
	return config.Tuning{
		HITLTimeout:        5 * time.Second, // short for tests
		HITLReminderBefore: 2 * time.Second,
	}
}

// TestRequestPermissionStoresPending verifies that RequestPermission writes a
// hitl_pending record with status "pending" before returning.
func TestRequestPermissionStoresPending(t *testing.T) {
	svc, _, s, _ := newTestService(t, defaultTuning())

	decisionCh := make(chan hitl.HITLDecision, 1)
	req := hitl.PermissionRequest{
		ChatID:     100,
		ID:         "permission_test01",
		SessionID:  "sess_01",
		Permission: "edit",
		Patterns:   []string{"/tmp/foo.go"},
		DecisionCh: decisionCh,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the service so the channel.Receive loop is active.
	errCh := make(chan error, 1)
	go func() { errCh <- svc.Run(ctx) }()
	time.Sleep(20 * time.Millisecond) // let Run() start

	if err := svc.RequestPermission(ctx, req); err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}

	// Verify SQLite has the pending record.
	pending, err := s.ListPendingHITL()
	if err != nil {
		t.Fatalf("ListPendingHITL: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending record, got %d", len(pending))
	}
	if pending[0].ID != "permission_test01" {
		t.Errorf("pending ID: got %q, want permission_test01", pending[0].ID)
	}
}

// TestRequestPermissionSendsKeyboard verifies that RequestPermission sends a keyboard
// via the channel.
func TestRequestPermissionSendsKeyboard(t *testing.T) {
	svc, ch, _, _ := newTestService(t, defaultTuning())

	decisionCh := make(chan hitl.HITLDecision, 1)
	req := hitl.PermissionRequest{
		ChatID:     100,
		ID:         "permission_kbd01",
		SessionID:  "sess_01",
		Permission: "run",
		Patterns:   []string{"/tmp/script.sh"},
		DecisionCh: decisionCh,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- svc.Run(ctx) }()
	time.Sleep(20 * time.Millisecond)

	if err := svc.RequestPermission(ctx, req); err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}

	// Allow up to 100ms for the keyboard to be sent.
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := ch.lastKeyboard(); ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	kbd, ok := ch.lastKeyboard()
	if !ok {
		t.Fatal("expected keyboard to be sent, but none was")
	}
	if len(kbd.Rows) != 4 {
		t.Errorf("keyboard rows: got %d, want 4", len(kbd.Rows))
	}
}

// TestCallbackAllowOnce verifies that a "hitl:<id>:once" callback resolves the
// PermissionRequest with Allow=true, Always=false and updates SQLite.
func TestCallbackAllowOnce(t *testing.T) {
	svc, ch, s, _ := newTestService(t, defaultTuning())

	decisionCh := make(chan hitl.HITLDecision, 1)
	req := hitl.PermissionRequest{
		ChatID:     100,
		ID:         "permission_once01",
		SessionID:  "sess_01",
		Permission: "edit",
		Patterns:   []string{"/tmp/a.go"},
		DecisionCh: decisionCh,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go svc.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	if err := svc.RequestPermission(ctx, req); err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	time.Sleep(20 * time.Millisecond) // let keyboard be sent

	// Inject the callback.
	ch.inject(channel.InboundMessage{
		ChatID:       100,
		CallbackData: "hitl:permission_once01:once",
	})

	select {
	case d := <-decisionCh:
		if !d.Allow {
			t.Error("expected Allow=true for 'once' callback")
		}
		if d.Always {
			t.Error("expected Always=false for 'once' callback")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for decision on 'once' callback")
	}

	// SQLite status should be updated to "resolved".
	pending, err := s.ListPendingHITL()
	if err != nil {
		t.Fatalf("ListPendingHITL: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after resolve, got %d", len(pending))
	}
}

// TestCallbackReject verifies that "hitl:<id>:reject" resolves with Allow=false.
func TestCallbackReject(t *testing.T) {
	svc, ch, _, _ := newTestService(t, defaultTuning())

	decisionCh := make(chan hitl.HITLDecision, 1)
	req := hitl.PermissionRequest{
		ChatID:     100,
		ID:         "permission_rej01",
		SessionID:  "sess_01",
		Permission: "edit",
		Patterns:   []string{"/tmp/b.go"},
		DecisionCh: decisionCh,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go svc.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	if err := svc.RequestPermission(ctx, req); err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	ch.inject(channel.InboundMessage{
		ChatID:       100,
		CallbackData: "hitl:permission_rej01:reject",
	})

	select {
	case d := <-decisionCh:
		if d.Allow {
			t.Error("expected Allow=false for 'reject' callback")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for decision on 'reject' callback")
	}
}

// TestDrainPendingSendsHITLDecisionDeny verifies that DrainPending sends
// HITLDecision{Allow: false} on all registered in-flight channels.
func TestDrainPendingSendsHITLDecisionDeny(t *testing.T) {
	svc, _, _, _ := newTestService(t, defaultTuning())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go svc.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Register two in-flight requests.
	ch1 := make(chan hitl.HITLDecision, 1)
	ch2 := make(chan hitl.HITLDecision, 1)

	svc.RequestPermission(ctx, hitl.PermissionRequest{ //nolint:errcheck
		ChatID: 100, ID: "permission_drain01", Permission: "edit",
		Patterns: []string{"/a"}, DecisionCh: ch1,
	})
	svc.RequestPermission(ctx, hitl.PermissionRequest{ //nolint:errcheck
		ChatID: 100, ID: "permission_drain02", Permission: "run",
		Patterns: []string{"/b"}, DecisionCh: ch2,
	})
	time.Sleep(20 * time.Millisecond)

	// DrainPending must send deny on both channels.
	svc.DrainPending()

	for _, chPair := range []struct {
		name string
		ch   <-chan hitl.HITLDecision
	}{{"ch1", ch1}, {"ch2", ch2}} {
		select {
		case d := <-chPair.ch:
			if d.Allow {
				t.Errorf("%s: expected Allow=false from DrainPending, got Allow=true", chPair.name)
			}
		case <-time.After(200 * time.Millisecond):
			t.Errorf("%s: timed out waiting for decision from DrainPending", chPair.name)
		}
	}
}

// TestStartupAutoRejectUpdatesSQLite verifies that on Run() startup, any hitl_pending
// records with status "pending" are updated to "auto_rejected" in SQLite.
// (The in-memory sync.Map is empty at startup so no channel send occurs — only SQLite.)
func TestStartupAutoRejectUpdatesSQLite(t *testing.T) {
	tuning := defaultTuning()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	// Pre-populate two stale pending records (as if from a previous run).
	if err := s.InsertHITLPending("stale_001", "opencode", "write_file"); err != nil {
		t.Fatalf("InsertHITLPending stale_001: %v", err)
	}
	if err := s.InsertHITLPending("stale_002", "claudecode", "bash"); err != nil {
		t.Fatalf("InsertHITLPending stale_002: %v", err)
	}

	ch := newMockChannel()
	svc := hitl.NewService(ch, s, tuning, nil) // replier=nil: no questions tested here

	ctx, cancel := context.WithCancel(context.Background())

	// Run the service briefly; startup auto-reject happens before the event loop.
	errCh := make(chan error, 1)
	go func() { errCh <- svc.Run(ctx) }()
	time.Sleep(50 * time.Millisecond) // let startup complete
	cancel()
	<-errCh

	// Both stale records should now be "auto_rejected" (not "pending").
	pending, err := s.ListPendingHITL()
	if err != nil {
		t.Fatalf("ListPendingHITL: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after startup auto-reject, got %d", len(pending))
	}
}

// TestRequestQuestionSequential verifies that RequestQuestion processes each question
// in order, resolves option indices to labels, and calls QuestionReplier.ReplyQuestion
// with the correct label strings (not raw indices).
func TestRequestQuestionSequential(t *testing.T) {
	svc, ch, _, rep := newTestService(t, defaultTuning())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go svc.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	req := hitl.QuestionRequest{
		ChatID:    100,
		ID:        "question_seq01",
		SessionID: "sess_01",
		Questions: []hitl.Question{
			{
				Question: "Framework?",
				Options:  []hitl.Option{{Label: "testify"}, {Label: "stdlib"}},
			},
			{
				Question: "Coverage?",
				Options:  []hitl.Option{{Label: "yes"}, {Label: "no"}},
			},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		// RequestQuestion returns only error (design §7).
		// Answers are delivered via mockReplier.ReplyQuestion.
		errCh <- svc.RequestQuestion(ctx, req)
	}()

	// Wait for first question keyboard, then answer it.
	time.Sleep(50 * time.Millisecond)
	ch.inject(channel.InboundMessage{
		ChatID:       100,
		CallbackData: "hitl:question_seq01:opt:0", // choose "testify" (index 0)
	})

	// Wait for second question keyboard, then answer it.
	time.Sleep(50 * time.Millisecond)
	ch.inject(channel.InboundMessage{
		ChatID:       100,
		CallbackData: "hitl:question_seq01:opt:0", // choose "yes" (index 0)
	})

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RequestQuestion returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RequestQuestion to complete")
	}

	// Verify the replier was called with actual labels, not raw indices.
	call, ok := rep.lastCall()
	if !ok {
		t.Fatal("expected mockReplier.ReplyQuestion to be called")
	}
	if call.id != "question_seq01" {
		t.Errorf("ReplyQuestion id = %q, want question_seq01", call.id)
	}
	if len(call.answers) != 2 {
		t.Fatalf("expected 2 answer groups, got %d", len(call.answers))
	}
	if len(call.answers[0]) != 1 || call.answers[0][0] != "testify" {
		t.Errorf("answers[0] = %v, want [testify]", call.answers[0])
	}
	if len(call.answers[1]) != 1 || call.answers[1][0] != "yes" {
		t.Errorf("answers[1] = %v, want [yes]", call.answers[1])
	}
}
