package conversation_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/conversation"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/store"
)

func newTestManager(t *testing.T, windowTurns, summarizeAtTurns int) (conversation.Manager, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() }) //nolint:errcheck
	m := conversation.NewManager(s, windowTurns, summarizeAtTurns)
	return m, s
}

func TestLoad_Empty(t *testing.T) {
	m, _ := newTestManager(t, 20, 0)
	msgs, err := m.Load(42)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Load empty: got %d msgs, want 0", len(msgs))
	}
}

func TestSaveAndLoad(t *testing.T) {
	m, _ := newTestManager(t, 20, 0)
	if err := m.Save(1, "user", "hello"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := m.Save(1, "assistant", "world"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	msgs, err := m.Load(1)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("Load: got %d msgs, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("msg[0]: got %+v", msgs[0])
	}
}

func TestLoad_RespectsWindowTurns(t *testing.T) {
	m, _ := newTestManager(t, 2, 0) // windowTurns=2 → max 4 rows
	for i := 0; i < 10; i++ {
		if err := m.Save(1, "user", "msg"); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if err := m.Save(1, "assistant", "reply"); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	msgs, err := m.Load(1)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) != 4 {
		t.Errorf("Load window: got %d msgs, want 4 (windowTurns*2)", len(msgs))
	}
}

func TestMaybeSummarize_DisabledByDefault(t *testing.T) {
	m, s := newTestManager(t, 20, 0) // summarizeAtTurns=0
	for i := 0; i < 30; i++ {
		if err := m.Save(1, "user", "msg"); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	// Mock LLM that must NOT be called.
	llm := &failIfCalledProvider{t: t}
	err := m.MaybeSummarize(context.Background(), 1, llm)
	if err != nil {
		t.Fatalf("MaybeSummarize: %v", err)
	}
	count, _ := s.CountMessages(1)
	if count != 30 {
		t.Errorf("rows should be unchanged: got %d, want 30", count)
	}
}

func TestMaybeSummarize_BelowThreshold_NoOp(t *testing.T) {
	m, s := newTestManager(t, 20, 16)
	for i := 0; i < 10; i++ { // 10 < 16 threshold
		if err := m.Save(1, "user", "msg"); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	llm := &failIfCalledProvider{t: t}
	err := m.MaybeSummarize(context.Background(), 1, llm)
	if err != nil {
		t.Fatalf("MaybeSummarize below threshold: %v", err)
	}
	count, _ := s.CountMessages(1)
	if count != 10 {
		t.Errorf("rows should be unchanged: got %d, want 10", count)
	}
}

func TestMaybeSummarize_AtThreshold_Summarizes(t *testing.T) {
	m, _ := newTestManager(t, 20, 5)
	for i := 0; i < 5; i++ {
		if err := m.Save(1, "user", fmt.Sprintf("msg %d", i)); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if err := m.Save(1, "assistant", fmt.Sprintf("reply %d", i)); err != nil {
			t.Fatalf("Save: %v", err)
		}
	} // 10 rows ≥ 5
	llm := &stubSummaryProvider{summary: "old stuff summarized"}
	err := m.MaybeSummarize(context.Background(), 1, llm)
	if err != nil {
		t.Fatalf("MaybeSummarize: %v", err)
	}
	msgs, _ := m.Load(1)
	// Should be: 1 summary row + 4 recent rows = 5
	if len(msgs) != 5 {
		t.Errorf("after summarization: got %d msgs, want 5", len(msgs))
	}
	// Summary row should exist.
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "Summary") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a summary row after summarization")
	}
}

// failIfCalledProvider panics if Chat is called (used to assert LLM is NOT called).
type failIfCalledProvider struct{ t *testing.T }

func (f *failIfCalledProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	f.t.Fatal("LLM should not be called")
	return nil, nil
}
func (f *failIfCalledProvider) Name() string { return "fail-if-called" }

// stubSummaryProvider returns a fixed summary string.
type stubSummaryProvider struct{ summary string }

func (s *stubSummaryProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: s.summary}, nil
}
func (s *stubSummaryProvider) Name() string { return "stub-summary" }
