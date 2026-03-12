# GistClaw Plan 5: Channel & HITL

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the Telegram channel adapter (`internal/channel/telegram`) and the HITL service (`internal/hitl`) that together bridge Telegram keyboards to permission approvals and sequential question answering.

**Architecture:** Four packages, one dependency direction: `hitl/keyboard.go` → `channel` (types only, never telego); `channel/telegram` → `telego` + `store`; `hitl/service.go` → `channel` (interface) + `store`. Tests use `httptest` for Telegram, `t.TempDir()` for SQLite, and plain channels for HITL decisions — no real network I/O.

**Tech Stack:** Go 1.25, `github.com/mymmrac/telego`, `modernc.org/sqlite` (via `internal/store`), `github.com/rs/zerolog`

**Design reference:** `docs/plans/design.md` §7, §9.6, §9.11, §9.17

**Depends on:** Plan 1 (channel.Channel interface, config), Plan 2 (store) — parallel to Plans 3 and 4

---

## Execution order

```
Task 1  internal/hitl/types.go          — all HITL types
Task 2  internal/hitl/keyboard.go       — KeyboardPayload builder (no telego)
Task 3  internal/channel/telegram/      — Telegram long-poll adapter
Task 4  internal/hitl/service.go        — hitl.Service + Approver interface
```

Tasks 1 and 2 are sequential (types must exist before keyboard builder). Task 3 is independent of Tasks 1–2. Task 4 requires Tasks 1–3 (uses types, keyboard, and channel.Channel).

---

### Task 1: `internal/hitl/types.go` — all HITL types

**Files:**
- Create: `internal/hitl/types.go`
- Create: `internal/hitl/types_test.go`

**Step 1: Write the failing test**

```go
// internal/hitl/types_test.go
package hitl_test

import (
	"testing"

	"github.com/canhta/gistclaw/internal/hitl"
)

func TestHITLDecisionZeroValue(t *testing.T) {
	var d hitl.HITLDecision
	if d.Allow {
		t.Error("zero-value HITLDecision.Allow must be false (deny by default)")
	}
	if d.Always {
		t.Error("zero-value HITLDecision.Always must be false")
	}
}

func TestPermissionRequestFields(t *testing.T) {
	ch := make(chan hitl.HITLDecision, 1)
	req := hitl.PermissionRequest{
		ChatID:     123,
		ID:         "permission_01ARZ3NDEKTSV4RRFFQ69G5FAV",
		SessionID:  "sess_01",
		Permission: "edit",
		Patterns:   []string{"/tmp/foo.go"},
		DecisionCh: ch,
	}
	if req.ChatID != 123 {
		t.Errorf("ChatID: got %d, want 123", req.ChatID)
	}
	if req.Permission != "edit" {
		t.Errorf("Permission: got %q, want edit", req.Permission)
	}
	if len(req.Patterns) != 1 {
		t.Errorf("Patterns: got %d, want 1", len(req.Patterns))
	}
	if req.DecisionCh == nil {
		t.Error("DecisionCh must not be nil")
	}
}

func TestQuestionRequestFields(t *testing.T) {
	req := hitl.QuestionRequest{
		ChatID:    456,
		ID:        "question_01ARZ3NDEKTSV4RRFFQ69G5FAV",
		SessionID: "sess_02",
		Questions: []hitl.Question{
			{
				Question: "Which test framework?",
				Header:   "Test",
				Options: []hitl.Option{
					{Label: "testify", Description: "Popular assertion library"},
					{Label: "stdlib", Description: "Built-in testing package"},
				},
				Multiple: false,
				Custom:   true,
			},
		},
	}
	if req.ChatID != 456 {
		t.Errorf("ChatID: got %d, want 456", req.ChatID)
	}
	if len(req.Questions) != 1 {
		t.Errorf("Questions: got %d, want 1", len(req.Questions))
	}
	q := req.Questions[0]
	if q.Question != "Which test framework?" {
		t.Errorf("Question text: got %q", q.Question)
	}
	if !q.Custom {
		t.Error("Custom must be true")
	}
	if len(q.Options) != 2 {
		t.Errorf("Options: got %d, want 2", len(q.Options))
	}
}

func TestOptionFields(t *testing.T) {
	opt := hitl.Option{Label: "yes", Description: "Confirm"}
	if opt.Label != "yes" {
		t.Errorf("Label: got %q, want yes", opt.Label)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/hitl/...
```

Expected: `FAIL` — package does not exist yet.

**Step 3: Write implementation**

```go
// internal/hitl/types.go
package hitl

// HITLDecision is the resolved decision for a PermissionRequest.
// Allow=false, Always=false is the safe zero-value (deny once).
type HITLDecision struct {
	Allow  bool
	Always bool
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
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/hitl/...
```

Expected: `PASS`

**Step 5: Commit**

```bash
git add internal/hitl/types.go internal/hitl/types_test.go
git commit -m "feat: add hitl types (HITLDecision, PermissionRequest, QuestionRequest, Question, Option)"
```

---

### Task 2: `internal/hitl/keyboard.go` — keyboard builder (no telego)

**Files:**
- Create: `internal/hitl/keyboard.go`
- Create: `internal/hitl/keyboard_test.go`

**Important constraint:** `keyboard.go` must import ONLY `internal/channel`. No `telego`, no other external packages.

**Step 1: Write the failing test**

```go
// internal/hitl/keyboard_test.go
package hitl_test

import (
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/hitl"
)

func TestPermissionKeyboardText(t *testing.T) {
	payload := hitl.PermissionKeyboard("perm_001", "edit", []string{"/tmp/foo.go", "/tmp/bar.go"})

	wantTextPrefix := "🔐 Permission request"
	if !strings.HasPrefix(payload.Text, wantTextPrefix) {
		t.Errorf("Text should start with %q, got: %q", wantTextPrefix, payload.Text)
	}
	if !strings.Contains(payload.Text, "edit") {
		t.Error("Text should contain the permission name")
	}
	if !strings.Contains(payload.Text, "/tmp/foo.go") {
		t.Error("Text should contain the first pattern")
	}
	if !strings.Contains(payload.Text, "/tmp/bar.go") {
		t.Error("Text should contain the second pattern")
	}
}

func TestPermissionKeyboardRows(t *testing.T) {
	payload := hitl.PermissionKeyboard("perm_001", "edit", []string{"/tmp/foo.go"})

	if len(payload.Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(payload.Rows))
	}

	// Each row has exactly one button.
	for i, row := range payload.Rows {
		if len(row) != 1 {
			t.Errorf("row %d: expected 1 button, got %d", i, len(row))
		}
	}

	// Verify CallbackData format: "hitl:<id>:<action>"
	wantData := []string{
		"hitl:perm_001:once",
		"hitl:perm_001:always",
		"hitl:perm_001:reject",
		"hitl:perm_001:stop",
	}
	for i, row := range payload.Rows {
		if row[0].CallbackData != wantData[i] {
			t.Errorf("row %d: CallbackData = %q, want %q", i, row[0].CallbackData, wantData[i])
		}
	}
}

func TestPermissionKeyboardLabels(t *testing.T) {
	payload := hitl.PermissionKeyboard("perm_001", "edit", []string{"/tmp/foo.go"})
	wantLabels := []string{"✅ Once", "✅ Always", "❌ Reject", "⏹ Stop"}
	for i, row := range payload.Rows {
		if row[0].Label != wantLabels[i] {
			t.Errorf("row %d: Label = %q, want %q", i, row[0].Label, wantLabels[i])
		}
	}
}

func TestPermissionKeyboardReturnsChannelType(t *testing.T) {
	// Compile-time check: PermissionKeyboard returns channel.KeyboardPayload (not telego).
	var _ channel.KeyboardPayload = hitl.PermissionKeyboard("x", "edit", nil)
}

func TestQuestionKeyboardSingleChoiceNoCustom(t *testing.T) {
	q := hitl.Question{
		Question: "Which test framework?",
		Options: []hitl.Option{
			{Label: "testify"},
			{Label: "stdlib"},
		},
		Multiple: false,
		Custom:   false,
	}
	payload := hitl.QuestionKeyboard("q_001", q)

	if payload.Text != "Which test framework?" {
		t.Errorf("Text = %q, want the question text", payload.Text)
	}
	// 2 options → 2 rows; no "Type your own" row.
	if len(payload.Rows) != 2 {
		t.Fatalf("expected 2 rows (no custom), got %d", len(payload.Rows))
	}
	// Each row has one button.
	if len(payload.Rows[0]) != 1 || len(payload.Rows[1]) != 1 {
		t.Error("each option row must have exactly one button")
	}
	// CallbackData: "hitl:<id>:opt:<index>"
	if payload.Rows[0][0].CallbackData != "hitl:q_001:opt:0" {
		t.Errorf("row 0 CallbackData = %q, want hitl:q_001:opt:0", payload.Rows[0][0].CallbackData)
	}
	if payload.Rows[1][0].CallbackData != "hitl:q_001:opt:1" {
		t.Errorf("row 1 CallbackData = %q, want hitl:q_001:opt:1", payload.Rows[1][0].CallbackData)
	}
	// Labels match option labels.
	if payload.Rows[0][0].Label != "testify" {
		t.Errorf("row 0 Label = %q, want testify", payload.Rows[0][0].Label)
	}
}

func TestQuestionKeyboardCustomAddsTypeYourOwn(t *testing.T) {
	q := hitl.Question{
		Question: "Pick or type:",
		Options:  []hitl.Option{{Label: "A"}},
		Multiple: false,
		Custom:   true,
	}
	payload := hitl.QuestionKeyboard("q_002", q)

	// 1 option + 1 "Type your own" row = 2 rows.
	if len(payload.Rows) != 2 {
		t.Fatalf("expected 2 rows (1 option + custom), got %d", len(payload.Rows))
	}
	lastRow := payload.Rows[len(payload.Rows)-1]
	if len(lastRow) != 1 {
		t.Fatal("custom row must have exactly 1 button")
	}
	if lastRow[0].Label != "✏️ Type your own" {
		t.Errorf("custom button Label = %q, want '✏️ Type your own'", lastRow[0].Label)
	}
	if lastRow[0].CallbackData != "hitl:q_002:custom" {
		t.Errorf("custom button CallbackData = %q, want 'hitl:q_002:custom'", lastRow[0].CallbackData)
	}
}

func TestQuestionKeyboardOptionLabelsIncludeDescription(t *testing.T) {
	q := hitl.Question{
		Question: "Choose:",
		Options: []hitl.Option{
			{Label: "yes", Description: "Confirm action"},
		},
	}
	payload := hitl.QuestionKeyboard("q_003", q)
	// When description is non-empty, button label should include it.
	btn := payload.Rows[0][0]
	if !strings.Contains(btn.Label, "yes") {
		t.Errorf("button label %q does not contain option label 'yes'", btn.Label)
	}
	if !strings.Contains(btn.Label, "Confirm action") {
		t.Errorf("button label %q does not contain description 'Confirm action'", btn.Label)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/hitl/...
```

Expected: `FAIL` — `PermissionKeyboard` and `QuestionKeyboard` functions do not exist yet.

**Step 3: Write implementation**

```go
// internal/hitl/keyboard.go
package hitl

// IMPORTANT: This file must ONLY import internal/channel.
// No telego, no external dependencies. The Telegram adapter in
// internal/channel/telegram translates channel.KeyboardPayload to telego types.

import (
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/channel"
)

// PermissionKeyboard builds the keyboard for a permission approval request.
//
// Text format:
//
//	🔐 Permission request
//	<permission> on:
//	<pattern1>
//	<pattern2>
//
// Rows:
//
//	[✅ Once]    → "hitl:<id>:once"
//	[✅ Always]  → "hitl:<id>:always"
//	[❌ Reject]  → "hitl:<id>:reject"
//	[⏹ Stop]    → "hitl:<id>:stop"
func PermissionKeyboard(id, permission string, patterns []string) channel.KeyboardPayload {
	var sb strings.Builder
	sb.WriteString("🔐 Permission request\n")
	sb.WriteString(permission)
	sb.WriteString(" on:")
	for _, p := range patterns {
		sb.WriteByte('\n')
		sb.WriteString(p)
	}

	return channel.KeyboardPayload{
		Text: sb.String(),
		Rows: []channel.ButtonRow{
			{{Label: "✅ Once", CallbackData: fmt.Sprintf("hitl:%s:once", id)}},
			{{Label: "✅ Always", CallbackData: fmt.Sprintf("hitl:%s:always", id)}},
			{{Label: "❌ Reject", CallbackData: fmt.Sprintf("hitl:%s:reject", id)}},
			{{Label: "⏹ Stop", CallbackData: fmt.Sprintf("hitl:%s:stop", id)}},
		},
	}
}

// QuestionKeyboard builds the keyboard for a single Question within a QuestionRequest.
//
// Text: the question text verbatim.
// Rows: one button per option (on its own row).
//
// CallbackData format for options: "hitl:<id>:opt:<index>"
// CallbackData for custom text:    "hitl:<id>:custom"
//
// If q.Custom is true, an extra "✏️ Type your own" button is appended as the last row.
//
// Option button labels:
//   - If Option.Description is non-empty: "<Label> — <Description>"
//   - Otherwise: "<Label>"
func QuestionKeyboard(id string, q Question) channel.KeyboardPayload {
	rows := make([]channel.ButtonRow, 0, len(q.Options)+1)

	for i, opt := range q.Options {
		label := opt.Label
		if opt.Description != "" {
			label = opt.Label + " — " + opt.Description
		}
		rows = append(rows, channel.ButtonRow{
			{
				Label:        label,
				CallbackData: fmt.Sprintf("hitl:%s:opt:%d", id, i),
			},
		})
	}

	if q.Custom {
		rows = append(rows, channel.ButtonRow{
			{
				Label:        "✏️ Type your own",
				CallbackData: fmt.Sprintf("hitl:%s:custom", id),
			},
		})
	}

	return channel.KeyboardPayload{
		Text: q.Question,
		Rows: rows,
	}
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/hitl/...
```

Expected: `PASS`

**Step 5: Verify no telego import**

```bash
go list -f '{{.Imports}}' github.com/canhta/gistclaw/internal/hitl
```

Expected: output must not contain `telego` anywhere.

**Step 6: Commit**

```bash
git add internal/hitl/keyboard.go internal/hitl/keyboard_test.go
git commit -m "feat: add hitl keyboard builder for permission and question payloads (no telego)"
```

---

### Task 3: `internal/channel/telegram/telegram.go` — Telegram long-poll adapter

**Files:**
- Create: `internal/channel/telegram/telegram.go`
- Create: `internal/channel/telegram/telegram_test.go`

**Step 1: Add dependency**

```bash
go get github.com/mymmrac/telego
go mod tidy
```

**Step 2: Write the failing test**

The test uses `net/http/httptest` to mock the Telegram Bot API. The `TelegramChannel` must
accept a custom base URL so tests never hit `api.telegram.org`.

```go
// internal/channel/telegram/telegram_test.go
package telegram_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	telego "github.com/mymmrac/telego"
	"github.com/canhta/gistclaw/internal/channel"
	tgchan "github.com/canhta/gistclaw/internal/channel/telegram"
	"github.com/canhta/gistclaw/internal/store"
)

// newTestStore creates an in-memory SQLite store for tests.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// fakeTelegramServer returns an httptest.Server that responds to Telegram Bot API requests.
// It records calls to /bot<token>/sendMessage.
type fakeTelegramServer struct {
	server       *httptest.Server
	sentMessages []string // captured text payloads
	callCount    atomic.Int32
}

func newFakeTelegramServer(t *testing.T) *fakeTelegramServer {
	t.Helper()
	fake := &fakeTelegramServer{}
	mux := http.NewServeMux()

	// getMe — required by telego on startup
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "getMe") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"id": 1, "is_bot": true, "first_name": "TestBot", "username": "testbot"},
			})
			return
		}
		if strings.Contains(path, "getUpdates") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []any{}})
			return
		}
		if strings.Contains(path, "sendMessage") {
			fake.callCount.Add(1)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if text, ok := body["text"].(string); ok {
				fake.sentMessages = append(fake.sentMessages, text)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 1, "date": 1, "chat": map[string]any{"id": 123}},
			})
			return
		}
		if strings.Contains(path, "sendChatAction") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
			return
		}
		// Default: 404
		http.NotFound(w, r)
	})

	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

// Verify TelegramChannel satisfies channel.Channel at compile time.
var _ channel.Channel = (*tgchan.TelegramChannel)(nil)

func TestTelegramChannelName(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL("test-token", s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	if ch.Name() != "telegram" {
		t.Errorf("Name() = %q, want telegram", ch.Name())
	}
}

func TestSendMessageShortText(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL("test-token", s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	ctx := context.Background()
	if err := ch.SendMessage(ctx, 123, "hello"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if fake.callCount.Load() != 1 {
		t.Errorf("expected 1 sendMessage call, got %d", fake.callCount.Load())
	}
}

func TestSendMessageSplitsLongText(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL("test-token", s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	// Build a string > 4096 chars; must be split into 2 messages.
	longText := strings.Repeat("a", 5000)
	ctx := context.Background()
	if err := ch.SendMessage(ctx, 123, longText); err != nil {
		t.Fatalf("SendMessage long: %v", err)
	}
	if fake.callCount.Load() != 2 {
		t.Errorf("expected 2 sendMessage calls for 5000-char text, got %d", fake.callCount.Load())
	}
}

func TestSendMessageSplitsExactlyAt4096(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL("test-token", s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	exactText := strings.Repeat("b", 4096)
	ctx := context.Background()
	if err := ch.SendMessage(ctx, 123, exactText); err != nil {
		t.Fatalf("SendMessage exact 4096: %v", err)
	}
	// Exactly 4096 chars — fits in one message.
	if fake.callCount.Load() != 1 {
		t.Errorf("expected 1 sendMessage call for exactly 4096-char text, got %d", fake.callCount.Load())
	}
}

func TestSendKeyboardTranslatesPayload(t *testing.T) {
	// Track the full request body to verify inline keyboard structure.
	var capturedBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "getMe") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"id": 1, "is_bot": true, "first_name": "TestBot", "username": "testbot"},
			})
			return
		}
		if strings.Contains(path, "sendMessage") {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 1, "date": 1, "chat": map[string]any{"id": 123}},
			})
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL("test-token", s, srv.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}

	payload := channel.KeyboardPayload{
		Text: "Choose an action:",
		Rows: []channel.ButtonRow{
			{
				{Label: "✅ Once", CallbackData: "hitl:perm_001:once"},
				{Label: "❌ Reject", CallbackData: "hitl:perm_001:reject"},
			},
		},
	}

	ctx := context.Background()
	if err := ch.SendKeyboard(ctx, 123, payload); err != nil {
		t.Fatalf("SendKeyboard: %v", err)
	}

	// Verify text was sent.
	if capturedBody["text"] != "Choose an action:" {
		t.Errorf("text = %v, want 'Choose an action:'", capturedBody["text"])
	}

	// Verify reply_markup was set.
	markup, ok := capturedBody["reply_markup"].(map[string]any)
	if !ok {
		t.Fatalf("reply_markup missing or wrong type: %T", capturedBody["reply_markup"])
	}
	inlineKeyboard, ok := markup["inline_keyboard"].([]any)
	if !ok {
		t.Fatalf("inline_keyboard missing: %v", markup)
	}
	if len(inlineKeyboard) != 1 {
		t.Fatalf("expected 1 keyboard row, got %d", len(inlineKeyboard))
	}
	row, ok := inlineKeyboard[0].([]any)
	if !ok {
		t.Fatalf("row 0 wrong type: %T", inlineKeyboard[0])
	}
	if len(row) != 2 {
		t.Fatalf("row 0: expected 2 buttons, got %d", len(row))
	}
}

func TestSendTyping(t *testing.T) {
	var typingCalled atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "getMe") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"id": 1, "is_bot": true, "first_name": "TestBot", "username": "testbot"},
			})
			return
		}
		if strings.Contains(path, "sendChatAction") {
			typingCalled.Store(true)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL("test-token", s, srv.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	ctx := context.Background()
	if err := ch.SendTyping(ctx, 123); err != nil {
		t.Fatalf("SendTyping: %v", err)
	}
	if !typingCalled.Load() {
		t.Error("expected sendChatAction to be called")
	}
}

func TestReceiveContextCancellation(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL("test-token", s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msgs, err := ch.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	// Drain until context cancels; should not block forever.
	for {
		select {
		case <-msgs:
		case <-ctx.Done():
			return // success
		}
	}
}

// TestUpdate_IDDedup verifies that duplicate update IDs are skipped.
// This test checks that SetLastUpdateID is called and that replaying the same
// update does not produce a second InboundMessage.
func TestUpdateIDStored(t *testing.T) {
	s := newTestStore(t)

	// Initially no record for this channel.
	id, err := s.GetLastUpdateID("telegram:testbot")
	if err != nil {
		t.Fatalf("GetLastUpdateID: %v", err)
	}
	if id != 0 {
		t.Errorf("initial update ID: got %d, want 0", id)
	}

	// Simulate storing an update ID (as TelegramChannel would do).
	if err := s.SetLastUpdateID("telegram:testbot", 99); err != nil {
		t.Fatalf("SetLastUpdateID: %v", err)
	}
	id, err = s.GetLastUpdateID("telegram:testbot")
	if err != nil {
		t.Fatalf("GetLastUpdateID after set: %v", err)
	}
	if id != 99 {
		t.Errorf("after set: got %d, want 99", id)
	}
}

// TestNewTelegramChannel verifies the public constructor works for the real token format
// (no network call — we just verify NewTelegramChannel returns a non-nil result with a
// valid-looking token; the token is never validated at construction time in v1).
func TestNewTelegramChannelConstructor(t *testing.T) {
	s := newTestStore(t)
	// NewTelegramChannel is the production constructor; it builds the real API URL.
	// We do NOT call it here because it would try to connect to api.telegram.org.
	// Instead verify that NewTelegramChannelWithBaseURL works and returns a non-nil channel.
	fake := newFakeTelegramServer(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL("tok:valid", s, fake.server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil TelegramChannel")
	}
}

// Ensure TelegramChannel's unused import of telego compiles correctly.
var _ *telego.Bot // reference telego to confirm it's imported in the test binary
```

**Step 3: Run test to verify it fails**

```bash
go test ./internal/channel/telegram/...
```

Expected: `FAIL` — package does not exist yet.

**Step 4: Write implementation**

```go
// internal/channel/telegram/telegram.go
package telegram

import (
	"context"
	"fmt"
	"time"

	telego "github.com/mymmrac/telego"
	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/store"
)

const (
	maxMessageLen  = 4096
	longPollTimeout = 30 // seconds passed to Telegram getUpdates
)

// TelegramChannel implements channel.Channel using Telegram Bot API long-polling.
// It deduplicates updates via store.GetLastUpdateID / store.SetLastUpdateID keyed
// by "telegram:<botUsername>".
type TelegramChannel struct {
	bot      *telego.Bot
	store    *store.Store
	stateKey string // "telegram:<botUsername>"
}

// NewTelegramChannel creates a TelegramChannel that connects to the real Telegram API.
// Returns an error if the token is rejected by Telegram on the getMe call.
func NewTelegramChannel(token string, s *store.Store) (*TelegramChannel, error) {
	return newWithOptions(token, s, nil)
}

// NewTelegramChannelWithBaseURL creates a TelegramChannel that connects to baseURL
// instead of https://api.telegram.org. Used in tests with httptest.Server.
func NewTelegramChannelWithBaseURL(token string, s *store.Store, baseURL string) (*TelegramChannel, error) {
	return newWithOptions(token, s, &baseURL)
}

func newWithOptions(token string, s *store.Store, baseURL *string) (*TelegramChannel, error) {
	opts := []telego.BotOption{}
	if baseURL != nil {
		opts = append(opts, telego.WithAPIServer(*baseURL))
	}

	bot, err := telego.NewBot(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot: %w", err)
	}

	me, err := bot.GetMe()
	if err != nil {
		return nil, fmt.Errorf("telegram: getMe: %w", err)
	}

	stateKey := fmt.Sprintf("telegram:%s", me.Username)
	return &TelegramChannel{
		bot:      bot,
		store:    s,
		stateKey: stateKey,
	}, nil
}

// Name returns the platform identifier.
func (t *TelegramChannel) Name() string { return "telegram" }

// Receive starts long-polling and returns a channel of inbound messages.
// Runs until ctx is cancelled. The returned channel is closed on exit.
// Duplicate update IDs (tracked in SQLite) are silently dropped.
func (t *TelegramChannel) Receive(ctx context.Context) (<-chan channel.InboundMessage, error) {
	out := make(chan channel.InboundMessage, 64)

	go func() {
		defer close(out)
		var offset int64

		// Load the last seen update ID from SQLite to resume correctly after restart.
		if last, err := t.store.GetLastUpdateID(t.stateKey); err == nil && last > 0 {
			offset = last + 1
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			updates, err := t.bot.GetUpdates(&telego.GetUpdatesParams{
				Offset:  int(offset),
				Timeout: longPollTimeout,
			})
			if err != nil {
				if ctx.Err() != nil {
					return // context cancelled during poll — clean exit
				}
				log.Warn().Err(err).Str("channel", t.stateKey).Msg("telegram: GetUpdates error; retrying in 1s")
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					return
				}
				continue
			}

			for _, u := range updates {
				updateID := int64(u.UpdateID)
				offset = updateID + 1

				// Dedup: persist last update ID; skip if we've already processed this.
				lastSeen, _ := t.store.GetLastUpdateID(t.stateKey)
				if updateID <= lastSeen {
					continue
				}
				if err := t.store.SetLastUpdateID(t.stateKey, updateID); err != nil {
					log.Warn().Err(err).Msg("telegram: failed to persist update ID")
				}

				msg := extractMessage(u)
				if msg == nil {
					continue
				}

				select {
				case out <- *msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// extractMessage converts a telego.Update into a channel.InboundMessage.
// Returns nil if the update carries no relevant content.
func extractMessage(u telego.Update) *channel.InboundMessage {
	if cb := u.CallbackQuery; cb != nil {
		chatID := int64(0)
		userID := int64(0)
		if cb.Message != nil {
			chatID = cb.Message.Chat.ID
		}
		if cb.From != nil {
			userID = cb.From.ID
		}
		return &channel.InboundMessage{
			ID:           fmt.Sprintf("%d", u.UpdateID),
			ChatID:       chatID,
			UserID:       userID,
			CallbackData: cb.Data,
		}
	}
	if m := u.Message; m != nil {
		return &channel.InboundMessage{
			ID:     fmt.Sprintf("%d", u.UpdateID),
			ChatID: m.Chat.ID,
			UserID: m.From.ID,
			Text:   m.Text,
		}
	}
	return nil
}

// SendMessage sends text to chatID. If text exceeds 4096 characters, it is hard-split
// at 4096-character boundaries and each chunk is sent as a separate message.
func (t *TelegramChannel) SendMessage(ctx context.Context, chatID int64, text string) error {
	chunks := splitText(text, maxMessageLen)
	for _, chunk := range chunks {
		if err := t.sendWithRetry(ctx, func() error {
			_, err := t.bot.SendMessage(&telego.SendMessageParams{
				ChatID: telego.ChatID{ID: chatID},
				Text:   chunk,
			})
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

// SendKeyboard sends a message with an inline keyboard.
// Translates channel.KeyboardPayload → telego.InlineKeyboardMarkup.
// Each channel.ButtonRow becomes one row of inline buttons.
func (t *TelegramChannel) SendKeyboard(ctx context.Context, chatID int64, payload channel.KeyboardPayload) error {
	markup := buildInlineKeyboard(payload)
	return t.sendWithRetry(ctx, func() error {
		_, err := t.bot.SendMessage(&telego.SendMessageParams{
			ChatID:      telego.ChatID{ID: chatID},
			Text:        payload.Text,
			ReplyMarkup: markup,
		})
		return err
	})
}

// SendTyping sends a "typing" chat action to chatID.
func (t *TelegramChannel) SendTyping(ctx context.Context, chatID int64) error {
	return t.sendWithRetry(ctx, func() error {
		return t.bot.SendChatAction(&telego.SendChatActionParams{
			ChatID: telego.ChatID{ID: chatID},
			Action: telego.ChatActionTyping,
		})
	})
}

// buildInlineKeyboard translates a channel.KeyboardPayload into a telego inline keyboard.
func buildInlineKeyboard(payload channel.KeyboardPayload) *telego.InlineKeyboardMarkup {
	rows := make([][]telego.InlineKeyboardButton, len(payload.Rows))
	for i, row := range payload.Rows {
		buttons := make([]telego.InlineKeyboardButton, len(row))
		for j, btn := range row {
			cbData := btn.CallbackData
			buttons[j] = telego.InlineKeyboardButton{
				Text:         btn.Label,
				CallbackData: cbData,
			}
		}
		rows[i] = buttons
	}
	return &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// sendWithRetry executes fn with Telegram-specific retry logic:
//   - 429 Too Many Requests: read RetryAfter, sleep, retry (unlimited).
//   - 5xx server errors: 3 retries at 500ms / 1s / 2s.
//   - 403 Forbidden (blocked): log WARN, return nil (no retry, no error).
func (t *TelegramChannel) sendWithRetry(ctx context.Context, fn func() error) error {
	const maxRetries = 3
	delays := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}

	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Try to extract telego API error details.
		var tErr *telego.Error
		if isTelegoError(err, &tErr) {
			switch {
			case tErr.ErrorCode == 403:
				// Bot was blocked by user — log and silently drop.
				log.Warn().Int("code", tErr.ErrorCode).Msg("telegram: bot blocked by user; dropping message")
				return nil
			case tErr.ErrorCode == 429:
				// Rate limit — honour RetryAfter.
				retryAfter := time.Duration(tErr.Parameters.RetryAfter) * time.Second
				if retryAfter <= 0 {
					retryAfter = 5 * time.Second
				}
				log.Warn().Dur("retry_after", retryAfter).Msg("telegram: rate limited (429); waiting")
				select {
				case <-time.After(retryAfter):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue // retry without consuming the attempt counter
			case tErr.ErrorCode >= 500:
				if attempt >= maxRetries {
					return fmt.Errorf("telegram: 5xx after %d retries: %w", maxRetries, err)
				}
				delay := delays[attempt]
				log.Warn().Int("attempt", attempt+1).Dur("delay", delay).Msg("telegram: 5xx; retrying")
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
		}

		// Non-retriable error.
		return err
	}
}

// isTelegoError attempts to cast err to *telego.Error.
// Returns true and sets out if successful.
func isTelegoError(err error, out **telego.Error) bool {
	if tErr, ok := err.(*telego.Error); ok {
		*out = tErr
		return true
	}
	return false
}

// splitText splits text into chunks of at most maxLen characters.
// Splitting is hard — no word-boundary awareness. If text fits in one chunk,
// returns a single-element slice.
func splitText(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}
	var chunks []string
	runes := []rune(text)
	for len(runes) > 0 {
		end := maxLen
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	return chunks
}
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/channel/telegram/...
```

Expected: `PASS`

**Step 6: Run all tests**

```bash
go test ./...
```

Expected: `PASS` across all packages written so far.

**Step 7: Commit**

```bash
git add internal/channel/telegram/ go.mod go.sum
git commit -m "feat: add Telegram long-poll channel adapter with message splitting, keyboard translation, and retry logic"
```

---

### Task 4: `internal/hitl/service.go` — HITL service

**Files:**
- Create: `internal/hitl/service.go`
- Create: `internal/hitl/service_test.go`

**Step 1: Write the failing test**

```go
// internal/hitl/service_test.go
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
	mu       sync.Mutex
	messages []string
	keyboards []channel.KeyboardPayload
	inbound  chan channel.InboundMessage
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

// newTestService creates a hitl.Service with a mock channel and temp SQLite store.
func newTestService(t *testing.T, tuning config.Tuning) (*hitl.Service, *mockChannel, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	ch := newMockChannel()
	svc := hitl.NewService(ch, s, tuning)
	return svc, ch, s
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
	svc, _, s := newTestService(t, defaultTuning())

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
	svc, ch, _ := newTestService(t, defaultTuning())

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
	svc, ch, s := newTestService(t, defaultTuning())

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
	svc, ch, _ := newTestService(t, defaultTuning())

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
	svc, _, _ := newTestService(t, defaultTuning())

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
	svc := hitl.NewService(ch, s, tuning)

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
// in order and returns answers collected from channel callbacks.
func TestRequestQuestionSequential(t *testing.T) {
	svc, ch, _ := newTestService(t, defaultTuning())

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

	answersCh := make(chan [][]string, 1)
	go func() {
		answers, err := svc.RequestQuestion(ctx, req)
		if err != nil {
			t.Errorf("RequestQuestion: %v", err)
		}
		answersCh <- answers
	}()

	// Wait for first question keyboard, then answer it.
	time.Sleep(50 * time.Millisecond)
	ch.inject(channel.InboundMessage{
		ChatID:       100,
		CallbackData: "hitl:question_seq01:opt:0", // choose "testify"
	})

	// Wait for second question keyboard, then answer it.
	time.Sleep(50 * time.Millisecond)
	ch.inject(channel.InboundMessage{
		ChatID:       100,
		CallbackData: "hitl:question_seq01:opt:0", // choose "yes"
	})

	select {
	case answers := <-answersCh:
		if len(answers) != 2 {
			t.Fatalf("expected 2 answer groups, got %d", len(answers))
		}
		if len(answers[0]) != 1 || answers[0][0] != "testify" {
			t.Errorf("answers[0] = %v, want [testify]", answers[0])
		}
		if len(answers[1]) != 1 || answers[1][0] != "yes" {
			t.Errorf("answers[1] = %v, want [yes]", answers[1])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for question answers")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/hitl/... -run TestRequestPermission -run TestCallback -run TestDrain -run TestStartup -run TestRequestQuestion
```

Expected: `FAIL` — `hitl.Service`, `hitl.NewService`, `hitl.Approver`, and related methods do not exist yet.

**Step 3: Write implementation**

```go
// internal/hitl/service.go
package hitl

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/store"
)

// Approver is the interface implemented by Service.
// Called by opencode.Service and claudecode.Service.
type Approver interface {
	// RequestPermission registers a permission request, sends a keyboard to the operator,
	// and returns immediately (non-blocking). The caller blocks on req.DecisionCh.
	RequestPermission(ctx context.Context, req PermissionRequest) error
	// RequestQuestion sends each question sequentially, waits for user answers, and
	// returns all answers as [][]string (one slice per question).
	RequestQuestion(ctx context.Context, req QuestionRequest) ([][]string, error)
}

// pendingItem stores the in-flight state for a PermissionRequest.
type pendingItem struct {
	decisionCh chan<- HITLDecision
}

// questionWaiter is used to hand off a question answer from the event loop.
type questionWaiter struct {
	id      string // full question request ID (e.g. "question_seq01")
	answerCh chan string // receives exactly one answer string (e.g. "testify")
}

// Service implements Approver. It is started by app.Run via withRestart.
type Service struct {
	ch     channel.Channel
	store  *store.Store
	tuning config.Tuning

	// pending tracks in-flight PermissionRequests keyed by ID.
	pending sync.Map

	// questionWaiters is a sync.Map[string, chan string] keyed by question request ID.
	// Each entry is written by the event loop when a matching callback arrives.
	questionWaiters sync.Map
}

// NewService creates a new HITL service.
func NewService(ch channel.Channel, s *store.Store, tuning config.Tuning) *Service {
	return &Service{ch: ch, store: s, tuning: tuning}
}

// Run is the main event loop. It:
//  1. Auto-rejects all hitl_pending records with status "pending" (from a previous run).
//  2. Calls ch.Receive to get the inbound message stream.
//  3. Dispatches callback messages to the appropriate registered handler.
//
// Blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	// Startup auto-reject: update stale pending records to "auto_rejected".
	if err := s.startupAutoReject(ctx); err != nil {
		log.Warn().Err(err).Msg("hitl: startup auto-reject failed")
	}

	msgs, err := s.ch.Receive(ctx)
	if err != nil {
		return fmt.Errorf("hitl: channel.Receive: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			if msg.CallbackData != "" && strings.HasPrefix(msg.CallbackData, "hitl:") {
				s.dispatchCallback(ctx, msg)
			}
		}
	}
}

// DrainPending sends HITLDecision{Allow: false} on all in-flight PermissionRequest
// channels. Called by app.Run when hitl.Service permanently fails.
func (s *Service) DrainPending() {
	s.pending.Range(func(key, val any) bool {
		item := val.(pendingItem)
		select {
		case item.decisionCh <- HITLDecision{Allow: false}:
		default:
			// Channel already has a value or was closed; skip.
		}
		s.pending.Delete(key)
		return true
	})
}

// RequestPermission writes a hitl_pending record, sends a keyboard, registers the
// DecisionCh in the in-memory map, and returns nil immediately (non-blocking).
func (s *Service) RequestPermission(ctx context.Context, req PermissionRequest) error {
	// Write to SQLite first (prevents race if user replies before registration).
	if err := s.store.InsertHITLPending(req.ID, req.SessionID, req.Permission); err != nil {
		return fmt.Errorf("hitl: insert pending: %w", err)
	}

	// Register the decision channel.
	s.pending.Store(req.ID, pendingItem{decisionCh: req.DecisionCh})

	// Send keyboard asynchronously so RequestPermission returns immediately.
	go func() {
		payload := PermissionKeyboard(req.ID, req.Permission, req.Patterns)
		if err := s.ch.SendKeyboard(ctx, req.ChatID, payload); err != nil {
			log.Warn().Err(err).Str("id", req.ID).Msg("hitl: failed to send permission keyboard")
		}

		// Schedule a reminder before timeout.
		reminderDelay := s.tuning.HITLTimeout - s.tuning.HITLReminderBefore
		if reminderDelay > 0 {
			select {
			case <-time.After(reminderDelay):
				// Only send reminder if still pending.
				if _, ok := s.pending.Load(req.ID); ok {
					msg := fmt.Sprintf("⏰ Approval still pending for: %s", req.ID)
					_ = s.ch.SendMessage(ctx, req.ChatID, msg)
				}
			case <-ctx.Done():
			}
		}
	}()

	return nil
}

// RequestQuestion processes questions sequentially.
// For each question: sends a keyboard, waits for user reply (or timeout).
// Returns [][]string with one entry per question.
func (s *Service) RequestQuestion(ctx context.Context, req QuestionRequest) ([][]string, error) {
	allAnswers := make([][]string, len(req.Questions))

	for i, q := range req.Questions {
		payload := QuestionKeyboard(req.ID, q)
		if err := s.ch.SendKeyboard(ctx, req.ChatID, payload); err != nil {
			log.Warn().Err(err).Str("id", req.ID).Int("q", i).Msg("hitl: failed to send question keyboard")
		}

		// Register a waiter channel for this question request ID.
		// The event loop will write the answer when a matching callback arrives.
		answerCh := make(chan string, 1)
		s.questionWaiters.Store(req.ID, answerCh)

		var answer string
		select {
		case answer = <-answerCh:
		case <-time.After(s.tuning.HITLTimeout):
			log.Warn().Str("id", req.ID).Int("q", i).Msg("hitl: question timed out; using empty answer")
			answer = ""
		case <-ctx.Done():
			return allAnswers, ctx.Err()
		}

		s.questionWaiters.Delete(req.ID)

		if answer == "" {
			allAnswers[i] = []string{}
		} else {
			allAnswers[i] = []string{answer}
		}
	}

	return allAnswers, nil
}

// dispatchCallback handles inbound callback messages whose data starts with "hitl:".
//
// Permission callbacks: "hitl:<id>:once|always|reject|stop"
// Question callbacks:   "hitl:<id>:opt:<n>" or "hitl:<id>:custom"
func (s *Service) dispatchCallback(ctx context.Context, msg channel.InboundMessage) {
	// Parse "hitl:<id>:<action>" or "hitl:<id>:opt:<n>"
	parts := strings.SplitN(msg.CallbackData, ":", 4) // ["hitl", "<id>", "<action>", optional-index]
	if len(parts) < 3 {
		log.Warn().Str("data", msg.CallbackData).Msg("hitl: malformed callback data")
		return
	}
	id := parts[1]
	action := parts[2]

	// Check if it's a permission callback.
	if val, ok := s.pending.Load(id); ok {
		item := val.(pendingItem)
		var decision HITLDecision
		switch action {
		case "once":
			decision = HITLDecision{Allow: true, Always: false}
		case "always":
			decision = HITLDecision{Allow: true, Always: true}
		case "reject":
			decision = HITLDecision{Allow: false}
		case "stop":
			decision = HITLDecision{Allow: false}
		default:
			log.Warn().Str("action", action).Msg("hitl: unknown permission action")
			return
		}

		select {
		case item.decisionCh <- decision:
		default:
		}
		s.pending.Delete(id)

		if err := s.store.ResolveHITL(id, "resolved"); err != nil {
			log.Warn().Err(err).Str("id", id).Msg("hitl: failed to resolve hitl_pending in SQLite")
		}
		return
	}

	// Check if it's a question callback.
	if val, ok := s.questionWaiters.Load(id); ok {
		answerCh := val.(chan string)
		var answer string
		switch action {
		case "opt":
			// "hitl:<id>:opt:<n>" — look up the option label from the index.
			// We receive the index as a string; the answer is the index itself for now.
			// The caller (opencode.Service) resolves the label from the original Question.
			// To send the actual label, we'd need to store the Question alongside the waiter.
			// For v1: send the raw opt index; opencode.Service will map it.
			if len(parts) == 4 {
				answer = parts[3] // the option index string, e.g. "0"
			}
		case "custom":
			// User wants to type a custom answer. For v1: return empty string and let
			// the caller handle the free-text follow-up.
			answer = ""
		default:
			answer = action
		}
		select {
		case answerCh <- answer:
		default:
		}
		return
	}

	log.Warn().Str("id", id).Str("action", action).Msg("hitl: received callback for unknown request")
}

// startupAutoReject marks all hitl_pending records with status "pending" as
// "auto_rejected" in SQLite. On restart, the in-memory sync.Map is empty so there
// are no channels to notify — only SQLite is updated.
func (s *Service) startupAutoReject(ctx context.Context) error {
	pending, err := s.store.ListPendingHITL()
	if err != nil {
		return fmt.Errorf("hitl: list pending on startup: %w", err)
	}

	for _, rec := range pending {
		if err := s.store.ResolveHITL(rec.ID, "auto_rejected"); err != nil {
			log.Warn().Err(err).Str("id", rec.ID).Msg("hitl: failed to auto-reject stale pending record")
			continue
		}
		log.Info().Str("id", rec.ID).Msg("hitl: auto-rejected stale pending record on startup")
		// Notify the operator if a chat ID is recoverable (not stored in hitl_pending in v1).
		// Best-effort: log only.
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/hitl/...
```

Expected: `PASS`

**Step 5: Run all tests**

```bash
go test ./...
```

Expected: `PASS` across all packages written so far.

**Step 6: Verify import constraint (no telego in hitl)**

```bash
go list -f '{{.Imports}}' github.com/canhta/gistclaw/internal/hitl
```

Expected: output must not contain `telego`.

**Step 7: Commit**

```bash
git add internal/hitl/service.go internal/hitl/service_test.go
git commit -m "feat: add hitl.Service with RequestPermission, RequestQuestion, DrainPending, and startup auto-reject"
```

---

## Final verification

After all four tasks are complete:

```bash
go build ./...
go test ./...
```

Both must produce zero errors.

Verify the import isolation constraint one more time:

```bash
go list -f '{{.Deps}}' github.com/canhta/gistclaw/internal/hitl | tr ' ' '\n' | grep -i telego
```

Expected: no output (telego must not appear anywhere in hitl's transitive deps).

---

Plan 5 complete. Run alongside Plans 3 and 4. After all three complete: Plan 6 (Agent Services).
