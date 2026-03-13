// internal/conversation/manager.go
package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/store"
)

// keepRecentRows is the number of recent rows preserved verbatim during summarization.
const keepRecentRows = 4

// Manager handles conversation history with optional proactive summarization.
type Manager interface {
	// Load returns history for chatID capped at windowTurns*2 rows (chronological order).
	Load(chatID int64) ([]providers.Message, error)

	// Save persists a single message row.
	Save(chatID int64, role, content string) error

	// MaybeSummarize checks if history exceeds the summarization threshold.
	// Fast path (below threshold or disabled): returns nil immediately.
	// Slow path: makes one LLM call and rewrites history in SQLite.
	// If summarizeAtTurns == 0, always returns nil.
	MaybeSummarize(ctx context.Context, chatID int64, llm providers.LLMProvider) error
}

type manager struct {
	store            *store.Store
	windowTurns      int
	summarizeAtTurns int
}

// NewManager constructs a Manager.
//
//	windowTurns: history cap in turns (rows fetched = windowTurns*2).
//	summarizeAtTurns: row count threshold; 0 = disabled.
func NewManager(s *store.Store, windowTurns, summarizeAtTurns int) Manager {
	return &manager{
		store:            s,
		windowTurns:      windowTurns,
		summarizeAtTurns: summarizeAtTurns,
	}
}

func (m *manager) Load(chatID int64) ([]providers.Message, error) {
	rows, err := m.store.GetHistory(chatID, m.windowTurns*2)
	if err != nil {
		return nil, fmt.Errorf("conversation: load: %w", err)
	}
	msgs := make([]providers.Message, len(rows))
	for i, r := range rows {
		msgs[i] = providers.Message{Role: r.Role, Content: r.Content}
	}
	return msgs, nil
}

func (m *manager) Save(chatID int64, role, content string) error {
	if err := m.store.SaveMessage(chatID, role, content); err != nil {
		return fmt.Errorf("conversation: save: %w", err)
	}
	return nil
}

func (m *manager) MaybeSummarize(ctx context.Context, chatID int64, llm providers.LLMProvider) error {
	if m.summarizeAtTurns <= 0 {
		return nil // disabled
	}
	if ctx.Err() != nil {
		return nil // clean shutdown
	}
	count, err := m.store.CountMessages(chatID)
	if err != nil {
		return fmt.Errorf("conversation: count: %w", err)
	}
	if count < m.summarizeAtTurns {
		return nil // below threshold — fast path
	}

	// Load all rows for summarization.
	rows, err := m.store.GetHistory(chatID, count)
	if err != nil {
		return fmt.Errorf("conversation: load for summarization: %w", err)
	}
	if len(rows) < keepRecentRows {
		return nil // too few rows to meaningfully summarize
	}

	// Partition: keep last keepRecentRows rows intact; summarize the rest.
	olderRows := rows[:len(rows)-keepRecentRows]
	recentRows := rows[len(rows)-keepRecentRows:]

	// Build summarization prompt.
	var sb strings.Builder
	for _, r := range olderRows {
		sb.WriteString(r.Role)
		sb.WriteString(": ")
		sb.WriteString(r.Content)
		sb.WriteString("\n")
	}
	prompt := "Summarize the following conversation history concisely, preserving all key facts, " +
		"decisions, preferences, and context. Return only the summary, no commentary.\n\n" +
		sb.String()

	resp, err := llm.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return fmt.Errorf("conversation: summarize LLM call: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("conversation: summarize: provider returned nil response")
	}

	summary := fmt.Sprintf("[Summary as of %s]: %s", time.Now().Format("2006-01-02"), resp.Content)
	newRows := make([]store.HistoryMessage, 0, 1+len(recentRows))
	newRows = append(newRows, store.HistoryMessage{Role: "assistant", Content: summary})
	for _, r := range recentRows {
		newRows = append(newRows, r)
	}

	if err := m.store.ReplaceHistory(chatID, newRows); err != nil {
		return fmt.Errorf("conversation: replace history: %w", err)
	}
	return nil
}
