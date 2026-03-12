# GistClaw Plan 6: Agent Services

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the two agent service packages (`internal/opencode` and `internal/claudecode`) and the `gistclaw-hook` helper binary that together allow GistClaw to control OpenCode via HTTP+SSE and Claude Code via subprocess+FSM from Telegram.

**Architecture:** Six files across three packages. `opencode.Service` drives `opencode serve` via HTTP and consumes SSE events. `claudecode.Service` spawns `claude -p` as a subprocess, parses stream-json output, and runs a local HTTP server (`127.0.0.1:8765`) that the `gistclaw-hook` helper binary calls back into for HITL decisions. Both services satisfy `infra.AgentHealthChecker` via their `Name()` + `IsAlive()` methods. No FSM is needed for OpenCode (server-side session state); a four-state FSM (`Idle → Running → WaitingInput → Idle`) governs Claude Code.

**Tech Stack:** Go 1.25, `net/http`, `os/exec`, `bufio`, `encoding/json` (all stdlib). No new external dependencies.

**Design reference:** `docs/plans/design.md` §4, §5 (Flows A–D, G), §9.5, §9.12–9.13, §11

**Depends on:** Plans 1–5 (all prior plans must be complete)

---

## Execution order

```
Task 1  internal/opencode/config.go + internal/opencode/stream.go   (SSE types + parser)
Task 2  internal/opencode/service.go                                 (OpenCode HTTP + SSE service)
Task 3  internal/claudecode/config.go + internal/claudecode/stream.go (stream-json types + parser)
Task 4  internal/claudecode/hooksrv.go                               (hook HTTP server)
Task 5  internal/claudecode/service.go                               (Claude Code subprocess + FSM)
Task 6  cmd/gistclaw-hook/main.go                                    (hook helper binary)
```

Tasks 1 and 3 produce only types/parsers with no external I/O — they can be written first.
Task 2 depends on Task 1. Task 4 depends on Task 3. Task 5 depends on Tasks 3 and 4.
Task 6 is fully standalone (reads stdin, POSTs HTTP, exits).

---

### Task 1: `internal/opencode/config.go` + `internal/opencode/stream.go` — SSE config types and event parser

**Files:**
- Create: `internal/opencode/config.go`
- Create: `internal/opencode/stream.go`
- Create: `internal/opencode/stream_test.go`

**Step 1: Write the failing test**

```go
// internal/opencode/stream_test.go
package opencode_test

import (
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/opencode"
)

func TestParseSSELine_TextPart(t *testing.T) {
	line := `data: {"type":"message.part.updated","part":{"type":"text","text":"hello"}}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.Type != "message.part.updated" {
		t.Errorf("Type: got %q, want message.part.updated", ev.Type)
	}
	if ev.Part == nil {
		t.Fatal("expected Part, got nil")
	}
	if ev.Part.Type != "text" {
		t.Errorf("Part.Type: got %q, want text", ev.Part.Type)
	}
	if ev.Part.Text != "hello" {
		t.Errorf("Part.Text: got %q, want hello", ev.Part.Text)
	}
}

func TestParseSSELine_StepFinish(t *testing.T) {
	line := `data: {"type":"message.part.updated","part":{"type":"step-finish","cost_usd":0.0042}}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Part == nil {
		t.Fatal("expected Part, got nil")
	}
	if ev.Part.Type != "step-finish" {
		t.Errorf("Part.Type: got %q, want step-finish", ev.Part.Type)
	}
	if ev.Part.CostUSD != 0.0042 {
		t.Errorf("Part.CostUSD: got %v, want 0.0042", ev.Part.CostUSD)
	}
}

func TestParseSSELine_PermissionAsked(t *testing.T) {
	line := `data: {"type":"permission.asked","id":"perm_01","session_id":"sess_01","permission":"edit","patterns":["/tmp/foo.go"]}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "permission.asked" {
		t.Errorf("Type: got %q, want permission.asked", ev.Type)
	}
	if ev.ID != "perm_01" {
		t.Errorf("ID: got %q, want perm_01", ev.ID)
	}
	if ev.Permission != "edit" {
		t.Errorf("Permission: got %q, want edit", ev.Permission)
	}
	if len(ev.Patterns) != 1 || ev.Patterns[0] != "/tmp/foo.go" {
		t.Errorf("Patterns: got %v, want [\"/tmp/foo.go\"]", ev.Patterns)
	}
}

func TestParseSSELine_QuestionAsked(t *testing.T) {
	line := `data: {"type":"question.asked","id":"q_01","session_id":"sess_01","questions":[{"question":"Which framework?","header":"Test","options":[{"label":"testify","description":"Popular"}],"multiple":false,"custom":true}]}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "question.asked" {
		t.Errorf("Type: got %q, want question.asked", ev.Type)
	}
	if len(ev.Questions) != 1 {
		t.Fatalf("Questions: got %d, want 1", len(ev.Questions))
	}
	q := ev.Questions[0]
	if !q.Custom {
		t.Error("Custom must be true")
	}
}

func TestParseSSELine_SessionIdle(t *testing.T) {
	line := `data: {"type":"session.status","status":{"type":"idle"}}`
	ev, err := opencode.ParseSSELine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "session.status" {
		t.Errorf("Type: got %q, want session.status", ev.Type)
	}
	if ev.Status == nil {
		t.Fatal("expected Status, got nil")
	}
	if ev.Status.Type != "idle" {
		t.Errorf("Status.Type: got %q, want idle", ev.Status.Type)
	}
}

func TestParseSSELine_NonDataLine(t *testing.T) {
	for _, line := range []string{"", ": keepalive", "event: ping"} {
		ev, err := opencode.ParseSSELine(line)
		if err != nil {
			t.Errorf("line %q: unexpected error: %v", line, err)
		}
		if ev != nil {
			t.Errorf("line %q: expected nil event, got %+v", line, ev)
		}
	}
}

func TestParseSSELine_MalformedJSON(t *testing.T) {
	line := "data: {not valid json"
	_, err := opencode.ParseSSELine(line)
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "SSE JSON") {
		t.Errorf("error should mention SSE JSON: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/opencode/...
```

Expected: `FAIL` — package does not exist yet.

**Step 3: Write implementation**

```go
// internal/opencode/config.go
package opencode

// Config holds all settings specific to the OpenCode service.
// Fields are populated by internal/config and injected at construction time.
type Config struct {
	// Port is the TCP port opencode serve will bind to (default 8766).
	Port int
	// Dir is the working directory passed to opencode serve --dir.
	Dir string
	// StartupTimeout is how long Run waits for the health endpoint after spawning
	// opencode serve (default 3s — callers should inject config.Tuning values).
	StartupTimeout int // seconds; kept as int to avoid importing time in config types
}
```

```go
// internal/opencode/stream.go
package opencode

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SSEOption is a single selectable answer in a Question received via SSE.
type SSEOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// SSEQuestion is a single question within a question.asked SSE event.
type SSEQuestion struct {
	Question string      `json:"question"`
	Header   string      `json:"header"`
	Options  []SSEOption `json:"options"`
	Multiple bool        `json:"multiple"`
	Custom   bool        `json:"custom"`
}

// SSEPart is the part payload inside a message.part.updated event.
type SSEPart struct {
	// Type is "text" or "step-finish" (and potentially others — unknown types are ignored).
	Type    string  `json:"type"`
	Text    string  `json:"text"`      // set when Type == "text"
	CostUSD float64 `json:"cost_usd"`  // set when Type == "step-finish"
}

// SSEStatus is the status payload inside a session.status event.
type SSEStatus struct {
	// Type is "idle", "running", etc.
	Type string `json:"type"`
}

// SSEEvent is the unified parsed form of a single SSE data line from OpenCode.
// Only the fields relevant to the observed event type are populated.
type SSEEvent struct {
	// Common fields
	Type      string `json:"type"`
	ID        string `json:"id"`         // permission.asked / question.asked
	SessionID string `json:"session_id"` // permission.asked / question.asked

	// message.part.updated
	Part *SSEPart `json:"part,omitempty"`

	// permission.asked
	Permission string   `json:"permission,omitempty"`
	Patterns   []string `json:"patterns,omitempty"`

	// question.asked
	Questions []SSEQuestion `json:"questions,omitempty"`

	// session.status
	Status *SSEStatus `json:"status,omitempty"`
}

// ParseSSELine parses a single line from an OpenCode SSE stream.
//
// Rules:
//   - Lines not starting with "data: " are silently skipped (returns nil, nil).
//     This includes blank lines, comment lines (": ..."), and "event:" lines.
//   - Lines starting with "data: " must contain valid JSON; malformed JSON returns an error.
//     The caller should log the error as WARN and skip the line rather than crashing.
func ParseSSELine(line string) (*SSEEvent, error) {
	const prefix = "data: "
	if !strings.HasPrefix(line, prefix) {
		return nil, nil
	}
	payload := line[len(prefix):]
	var ev SSEEvent
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return nil, fmt.Errorf("SSE JSON parse error: %w (line: %q)", err, line)
	}
	return &ev, nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/opencode/...
```

Expected: `PASS` for all stream tests.

**Step 5: Commit**

```bash
git add internal/opencode/config.go internal/opencode/stream.go internal/opencode/stream_test.go
git commit -m "feat(opencode): add SSE config types and event parser"
```

---

### Task 2: `internal/opencode/service.go` — OpenCode HTTP + SSE service

**Files:**
- Create: `internal/opencode/service.go`
- Create: `internal/opencode/service_test.go`

**Dependencies injected at construction (all interfaces, no concrete imports):**
- `channel.Channel` — for `SendMessage`
- `hitl.Approver` — for `RequestPermission` / `RequestQuestion`
- `infra.CostGuard` — for `Track`
- `infra.SOULLoader` — for `Load`

**Step 1: Write the failing test**

```go
// internal/opencode/service_test.go
package opencode_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/hitl"
	"github.com/canhta/gistclaw/internal/opencode"
)

// --- minimal fakes (no mock framework) ---

type fakeChannel struct {
	messages []string
}

func (f *fakeChannel) SendMessage(_ context.Context, _ int64, text string) error {
	f.messages = append(f.messages, text)
	return nil
}
func (f *fakeChannel) SendKeyboard(_ context.Context, _ int64, _ channel.KeyboardPayload) error {
	return nil
}
func (f *fakeChannel) SendTyping(_ context.Context, _ int64) error              { return nil }
func (f *fakeChannel) Name() string                                              { return "fake" }
func (f *fakeChannel) Receive(_ context.Context) (<-chan channel.InboundMessage, error) {
	return nil, nil
}

type fakeApprover struct{ called atomic.Bool }

func (a *fakeApprover) RequestPermission(_ context.Context, req hitl.PermissionRequest) error {
	a.called.Store(true)
	return nil
}
func (a *fakeApprover) RequestQuestion(_ context.Context, req hitl.QuestionRequest) error {
	a.called.Store(true)
	return nil
}

type fakeCostGuard struct{ tracked float64 }

func (g *fakeCostGuard) Track(usd float64) { g.tracked += usd }

type fakeSOULLoader struct{}

func (s *fakeSOULLoader) Load() (string, error) { return "you are a helpful agent", nil }

// --- helpers ---

// newMockOpenCodeServer returns an httptest.Server that simulates opencode serve.
// sseLines is a slice of raw SSE lines to emit from GET /event (one per 10ms).
// promptCalled is set to true when POST /session/:id/prompt_async is received.
func newMockOpenCodeServer(t *testing.T, sseLines []string, promptCalled *atomic.Bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("/global/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create session
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "sess_test_01"})
	})

	// Prompt async
	mux.HandleFunc("/session/sess_test_01/prompt_async", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if promptCalled != nil {
			promptCalled.Store(true)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// SSE event stream
	mux.HandleFunc("/event", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Log("ResponseWriter does not implement Flusher")
			return
		}
		for _, line := range sseLines {
			time.Sleep(10 * time.Millisecond)
			fmt.Fprintln(w, line)
			flusher.Flush()
		}
	})

	return httptest.NewServer(mux)
}

// --- tests ---

func TestIsAlive_HealthEndpointReturns200(t *testing.T) {
	srv := newMockOpenCodeServer(t, nil, nil)
	defer srv.Close()

	// Extract port from srv.URL
	parts := strings.Split(srv.URL, ":")
	portStr := parts[len(parts)-1]
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	svc := opencode.New(opencode.Config{Port: port, Dir: t.TempDir()},
		&fakeChannel{}, &fakeApprover{}, &fakeCostGuard{}, &fakeSOULLoader{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if !svc.IsAlive(ctx) {
		t.Error("IsAlive: expected true when health returns 200")
	}
}

func TestIsAlive_NoServer(t *testing.T) {
	svc := opencode.New(opencode.Config{Port: 19999, Dir: t.TempDir()},
		&fakeChannel{}, &fakeApprover{}, &fakeCostGuard{}, &fakeSOULLoader{})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if svc.IsAlive(ctx) {
		t.Error("IsAlive: expected false when no server is running")
	}
}

func TestName_ReturnsOpencode(t *testing.T) {
	svc := opencode.New(opencode.Config{Port: 8766, Dir: t.TempDir()},
		&fakeChannel{}, &fakeApprover{}, &fakeCostGuard{}, &fakeSOULLoader{})
	if svc.Name() != "opencode" {
		t.Errorf("Name: got %q, want opencode", svc.Name())
	}
}

func TestSubmitTask_StreamsTextToChannel(t *testing.T) {
	var promptCalled atomic.Bool
	sseLines := []string{
		`data: {"type":"message.part.updated","part":{"type":"text","text":"Hello, "}}`,
		`data: {"type":"message.part.updated","part":{"type":"text","text":"world!"}}`,
		`data: {"type":"session.status","status":{"type":"idle"}}`,
	}
	srv := newMockOpenCodeServer(t, sseLines, &promptCalled)
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	portStr := parts[len(parts)-1]
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	ch := &fakeChannel{}
	svc := opencode.New(opencode.Config{Port: port, Dir: t.TempDir()},
		ch, &fakeApprover{}, &fakeCostGuard{}, &fakeSOULLoader{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := svc.SubmitTask(ctx, 123, "build the auth module"); err != nil {
		t.Fatalf("SubmitTask: unexpected error: %v", err)
	}

	if !promptCalled.Load() {
		t.Error("expected POST /session/:id/prompt_async to be called")
	}

	// Should have sent text + "✅ Done"
	full := strings.Join(ch.messages, "")
	if !strings.Contains(full, "Hello, world!") {
		t.Errorf("expected channel to contain streamed text, got: %v", ch.messages)
	}
	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "✅ Done") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ✅ Done message, got: %v", ch.messages)
	}
}

func TestSubmitTask_ZeroOutput_SendsWarning(t *testing.T) {
	sseLines := []string{
		`data: {"type":"session.status","status":{"type":"idle"}}`,
	}
	srv := newMockOpenCodeServer(t, sseLines, nil)
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	portStr := parts[len(parts)-1]
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	ch := &fakeChannel{}
	svc := opencode.New(opencode.Config{Port: port, Dir: t.TempDir()},
		ch, &fakeApprover{}, &fakeCostGuard{}, &fakeSOULLoader{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := svc.SubmitTask(ctx, 123, "do something"); err != nil {
		t.Fatalf("SubmitTask: unexpected error: %v", err)
	}

	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "no output") || strings.Contains(m, "⚠️") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected zero-output warning, got: %v", ch.messages)
	}
}

func TestSubmitTask_BusyCheck_ReturnsWarning(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/global/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "sess_busy_01"})
	})
	mux.HandleFunc("/session/sess_busy_01/prompt_async", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"session is busy"}`, http.StatusInternalServerError)
	})
	// SSE endpoint (should not be reached in this test)
	mux.HandleFunc("/event", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// emit nothing
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	portStr := parts[len(parts)-1]
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	ch := &fakeChannel{}
	svc := opencode.New(opencode.Config{Port: port, Dir: t.TempDir()},
		ch, &fakeApprover{}, &fakeCostGuard{}, &fakeSOULLoader{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := svc.SubmitTask(ctx, 123, "another task")
	if err != nil {
		t.Fatalf("SubmitTask with busy server: unexpected non-nil error: %v", err)
	}

	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "busy") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected busy warning message, got: %v", ch.messages)
	}
}

func TestSubmitTask_CostTracked(t *testing.T) {
	sseLines := []string{
		`data: {"type":"message.part.updated","part":{"type":"step-finish","cost_usd":0.0042}}`,
		`data: {"type":"session.status","status":{"type":"idle"}}`,
	}
	srv := newMockOpenCodeServer(t, sseLines, nil)
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	portStr := parts[len(parts)-1]
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	guard := &fakeCostGuard{}
	svc := opencode.New(opencode.Config{Port: port, Dir: t.TempDir()},
		&fakeChannel{}, &fakeApprover{}, guard, &fakeSOULLoader{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := svc.SubmitTask(ctx, 123, "cost test"); err != nil {
		t.Fatalf("SubmitTask: unexpected error: %v", err)
	}
	if guard.tracked != 0.0042 {
		t.Errorf("CostGuard.Track: got %v, want 0.0042", guard.tracked)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/opencode/...
```

Expected: `FAIL` — `opencode.New` does not exist yet.

**Step 3: Write implementation**

```go
// internal/opencode/service.go
package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/hitl"
)

// Service is the interface satisfied by *serviceImpl.
// It also satisfies infra.AgentHealthChecker via Name() + IsAlive().
type Service interface {
	Run(ctx context.Context) error
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	Stop(ctx context.Context) error
	IsAlive(ctx context.Context) bool
	Name() string // returns "opencode"
}

type serviceImpl struct {
	cfg    Config
	ch     channel.Channel
	hitl   hitlApprover
	guard  costTracker
	soul   soulLoader
	client *http.Client

	mu        sync.Mutex
	sessionID string
	cmd       *exec.Cmd
}

// Narrow dependency interfaces — keeps serviceImpl testable without importing infra directly.
// The concrete infra types satisfy these interfaces; tests provide lightweight fakes.

type hitlApprover interface {
	RequestPermission(ctx context.Context, req hitl.PermissionRequest) error
	RequestQuestion(ctx context.Context, req hitl.QuestionRequest) error
}

type costTracker interface {
	Track(usd float64)
}

type soulLoader interface {
	Load() (string, error)
}

// New constructs a new opencode.Service. All dependencies are injected as interfaces.
// In production, pass *infra.CostGuard and *infra.SOULLoader (both satisfy the narrow
// interfaces above). In tests, pass lightweight fakes.
func New(cfg Config, ch channel.Channel, approver hitlApprover, guard costTracker, soul soulLoader) Service {
	return &serviceImpl{
		cfg:    cfg,
		ch:     ch,
		hitl:   approver,
		guard:  guard,
		soul:   soul,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *serviceImpl) Name() string { return "opencode" }

// Run starts opencode serve and blocks until ctx is cancelled or the subprocess exits.
func (s *serviceImpl) Run(ctx context.Context) error {
	args := []string{
		"serve",
		"--port", fmt.Sprintf("%d", s.cfg.Port),
		"--hostname", "127.0.0.1",
		"--dir", s.cfg.Dir,
	}
	cmd := exec.CommandContext(ctx, "opencode", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opencode: start subprocess: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	// Wait up to StartupTimeout seconds (default behaviour: 3s) for health check.
	timeout := s.cfg.StartupTimeout
	if timeout <= 0 {
		timeout = 3
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for time.Now().Before(deadline) {
		hctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		alive := s.isAliveURL(hctx)
		cancel()
		if alive {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !s.IsAlive(ctx) {
		_ = cmd.Process.Kill()
		return fmt.Errorf("opencode: failed to start within %ds", timeout)
	}

	return cmd.Wait()
}

// SubmitTask submits a prompt to the running OpenCode instance.
func (s *serviceImpl) SubmitTask(ctx context.Context, chatID int64, prompt string) error {
	// Ensure (or reuse) a session.
	sessionID, err := s.ensureSession(ctx)
	if err != nil {
		return fmt.Errorf("opencode: ensure session: %w", err)
	}

	// Load SOUL.md.
	soul, err := s.soul.Load()
	if err != nil {
		log.Warn().Err(err).Msg("opencode: could not load SOUL.md; proceeding without system prompt")
		soul = ""
	}

	// Build and send prompt_async request.
	body, _ := json.Marshal(map[string]interface{}{
		"parts":  []map[string]string{{"type": "text", "text": prompt}},
		"system": soul,
	})
	url := s.base() + "/session/" + sessionID + "/prompt_async"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("opencode: prompt_async: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusInternalServerError {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(bodyBytes), "is busy") {
			_ = s.ch.SendMessage(ctx, chatID, "⚠️ OpenCode is busy. Wait for current task to finish.")
			return nil
		}
		return fmt.Errorf("opencode: prompt_async returned HTTP 500: %s", bodyBytes)
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opencode: prompt_async returned HTTP %d: %s", resp.StatusCode, bodyBytes)
	}

	// Consume SSE stream.
	return s.consumeSSE(ctx, chatID, sessionID)
}

// Stop aborts the active session and kills the subprocess.
func (s *serviceImpl) Stop(ctx context.Context) error {
	s.mu.Lock()
	sessionID := s.sessionID
	cmd := s.cmd
	s.mu.Unlock()

	if sessionID != "" {
		abortURL := s.base() + "/session/" + sessionID + "/abort"
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, abortURL, nil)
		resp, err := s.client.Do(req)
		if err != nil {
			log.Warn().Err(err).Msg("opencode: abort session")
		} else {
			resp.Body.Close()
		}
		s.mu.Lock()
		s.sessionID = ""
		s.mu.Unlock()
	}

	if cmd != nil && cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil {
			log.Warn().Err(err).Msg("opencode: kill subprocess")
		}
	}
	return nil
}

// IsAlive checks whether the opencode serve health endpoint responds with 200.
func (s *serviceImpl) IsAlive(ctx context.Context) bool {
	return s.isAliveURL(ctx)
}

// --- private helpers ---

func (s *serviceImpl) base() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.cfg.Port)
}

func (s *serviceImpl) isAliveURL(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base()+"/global/health", nil)
	if err != nil {
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *serviceImpl) ensureSession(ctx context.Context) (string, error) {
	s.mu.Lock()
	existing := s.sessionID
	s.mu.Unlock()
	if existing != "" {
		return existing, nil
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.base()+"/session", nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode session response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("opencode: session response missing id")
	}

	s.mu.Lock()
	s.sessionID = result.ID
	s.mu.Unlock()
	return result.ID, nil
}

func (s *serviceImpl) consumeSSE(ctx context.Context, chatID int64, sessionID string) error {
	url := fmt.Sprintf("%s/event?directory=%s", s.base(), s.cfg.Dir)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "text/event-stream")

	// Use a client without timeout for the long-running SSE stream.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("opencode: SSE connect: %w", err)
	}
	defer resp.Body.Close()

	var buf strings.Builder // accumulates text output
	var hadOutput bool      // true once any text part has been received
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		ev, err := ParseSSELine(line)
		if err != nil {
			log.Warn().Err(err).Msg("opencode: skip malformed SSE line")
			continue
		}
		if ev == nil {
			continue
		}

		switch ev.Type {
		case "message.part.updated":
			if ev.Part == nil {
				continue
			}
			switch ev.Part.Type {
			case "text":
				hadOutput = true
				buf.WriteString(ev.Part.Text)
				// Flush to Telegram at 4096-char boundary.
				for buf.Len() >= 4096 {
					chunk := buf.String()[:4096]
					_ = s.ch.SendMessage(ctx, chatID, chunk)
					rest := buf.String()[4096:]
					buf.Reset()
					buf.WriteString(rest)
				}
			case "step-finish":
				s.guard.Track(ev.Part.CostUSD)
			}

		case "permission.asked":
			decisionCh := make(chan hitl.HITLDecision, 1)
			_ = s.hitl.RequestPermission(ctx, hitl.PermissionRequest{
				ChatID:     chatID,
				ID:         ev.ID,
				SessionID:  ev.SessionID,
				Permission: ev.Permission,
				Patterns:   ev.Patterns,
				DecisionCh: decisionCh,
			})

		case "question.asked":
			var questions []hitl.Question
			for _, q := range ev.Questions {
				var opts []hitl.Option
				for _, o := range q.Options {
					opts = append(opts, hitl.Option{Label: o.Label, Description: o.Description})
				}
				questions = append(questions, hitl.Question{
					Question: q.Question,
					Header:   q.Header,
					Options:  opts,
					Multiple: q.Multiple,
					Custom:   q.Custom,
				})
			}
			_ = s.hitl.RequestQuestion(ctx, hitl.QuestionRequest{
				ChatID:    chatID,
				ID:        ev.ID,
				SessionID: ev.SessionID,
				Questions: questions,
			})

		case "session.status":
			if ev.Status != nil && ev.Status.Type == "idle" {
				// Clear session.
				s.mu.Lock()
				s.sessionID = ""
				s.mu.Unlock()
				// Send completion or zero-output message.
				if !hadOutput {
					_ = s.ch.SendMessage(ctx, chatID, "⚠️ Agent finished but produced no output.")
				} else {
					// Flush any remaining buffered text.
					if buf.Len() > 0 {
						_ = s.ch.SendMessage(ctx, chatID, buf.String())
						buf.Reset()
					}
					_ = s.ch.SendMessage(ctx, chatID, "✅ Done")
				}
				return nil
			}
		}
	}
	return scanner.Err()
}

**Step 4: Run test to verify it passes**

```bash
go test ./internal/opencode/... -v -timeout 30s
```

Expected: `PASS` for all tests. If `TestSubmitTask_*` tests time out, check that the mock SSE server sends the `session.status idle` event and the scanner terminates.

**Step 5: Commit**

```bash
git add internal/opencode/service.go internal/opencode/service_test.go
git commit -m "feat(opencode): implement OpenCode HTTP+SSE service"
```

---

### Task 3: `internal/claudecode/config.go` + `internal/claudecode/stream.go` — stream-json config types and parser

**Files:**
- Create: `internal/claudecode/config.go`
- Create: `internal/claudecode/stream.go`
- Create: `internal/claudecode/stream_test.go`

**Step 1: Write the failing test**

```go
// internal/claudecode/stream_test.go
package claudecode_test

import (
	"testing"

	"github.com/canhta/gistclaw/internal/claudecode"
)

func TestParseStreamLine_Text(t *testing.T) {
	line := `{"type":"text","text":"Hello from claude"}`
	ev, err := claudecode.ParseStreamLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.Type != "text" {
		t.Errorf("Type: got %q, want text", ev.Type)
	}
	if ev.Text != "Hello from claude" {
		t.Errorf("Text: got %q, want 'Hello from claude'", ev.Text)
	}
}

func TestParseStreamLine_ResultSuccess(t *testing.T) {
	line := `{"type":"result","total_cost_usd":0.0123,"is_error":false,"result":"done"}`
	ev, err := claudecode.ParseStreamLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "result" {
		t.Errorf("Type: got %q, want result", ev.Type)
	}
	if ev.TotalCostUSD != 0.0123 {
		t.Errorf("TotalCostUSD: got %v, want 0.0123", ev.TotalCostUSD)
	}
	if ev.IsError {
		t.Error("IsError must be false")
	}
	if ev.Result != "done" {
		t.Errorf("Result: got %q, want done", ev.Result)
	}
}

func TestParseStreamLine_ResultError(t *testing.T) {
	line := `{"type":"result","total_cost_usd":0,"is_error":true,"result":"something went wrong"}`
	ev, err := claudecode.ParseStreamLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ev.IsError {
		t.Error("IsError must be true")
	}
}

func TestParseStreamLine_EmptyLine(t *testing.T) {
	ev, err := claudecode.ParseStreamLine("")
	if err != nil {
		t.Errorf("empty line: unexpected error: %v", err)
	}
	if ev != nil {
		t.Errorf("empty line: expected nil event, got %+v", ev)
	}
}

func TestParseStreamLine_MalformedJSON(t *testing.T) {
	_, err := claudecode.ParseStreamLine("{not json")
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

func TestParseStreamLine_UnknownType(t *testing.T) {
	// Unknown types must not error — just return the parsed event.
	ev, err := claudecode.ParseStreamLine(`{"type":"assistant","message":{}}`)
	if err != nil {
		t.Fatalf("unexpected error for unknown type: %v", err)
	}
	if ev == nil {
		t.Fatal("expected non-nil event for unknown type")
	}
	if ev.Type != "assistant" {
		t.Errorf("Type: got %q, want assistant", ev.Type)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/claudecode/...
```

Expected: `FAIL` — package does not exist yet.

**Step 3: Write implementation**

```go
// internal/claudecode/config.go
package claudecode

// Config holds all settings specific to the ClaudeCode service.
type Config struct {
	// Dir is the working directory for claude -p invocations.
	Dir string
	// HookServerAddr is the address the hook HTTP server listens on.
	// Default: "127.0.0.1:8765"
	HookServerAddr string
	// SettingsPath is the path to .claude/settings.local.json within Dir.
	// If empty, defaults to filepath.Join(Dir, ".claude/settings.local.json").
	SettingsPath string
}
```

```go
// internal/claudecode/stream.go
package claudecode

import (
	"encoding/json"
	"fmt"
	"strings"
)

// StreamEvent is the parsed form of a single newline-delimited JSON object from
// the `claude -p --output-format stream-json` stdout.
//
// Relevant types:
//   - "text"   — Text field holds the incremental content.
//   - "result" — TotalCostUSD and IsError hold the final cost and error status.
//   - All other types (e.g. "assistant", "user", "system") are passed through
//     unchanged; the caller may ignore unknown types.
type StreamEvent struct {
	Type         string  `json:"type"`
	Text         string  `json:"text"`           // "text"
	TotalCostUSD float64 `json:"total_cost_usd"` // "result"
	IsError      bool    `json:"is_error"`       // "result"
	Result       string  `json:"result"`         // "result" — final message or error detail
}

// ParseStreamLine parses a single line from `claude -p` stream-json stdout.
//
// Rules:
//   - Empty lines return (nil, nil) — callers must handle this gracefully.
//   - Non-empty lines must be valid JSON; malformed JSON returns an error.
//     The caller should log the error as WARN and skip (do NOT crash).
func ParseStreamLine(line string) (*StreamEvent, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	var ev StreamEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return nil, fmt.Errorf("stream-json parse error: %w (line: %q)", err, line)
	}
	return &ev, nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/claudecode/...
```

Expected: `PASS` for all stream tests.

**Step 5: Commit**

```bash
git add internal/claudecode/config.go internal/claudecode/stream.go internal/claudecode/stream_test.go
git commit -m "feat(claudecode): add stream-json config types and parser"
```

---

### Task 4: `internal/claudecode/hooksrv.go` — hook HTTP server

**Files:**
- Create: `internal/claudecode/hooksrv.go`
- Create: `internal/claudecode/hooksrv_test.go`

**Step 1: Write the failing test**

```go
// internal/claudecode/hooksrv_test.go
package claudecode_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/claudecode"
	"github.com/canhta/gistclaw/internal/hitl"
)

> **Note:** `"fmt"` is required for the `itoa` helper function defined at the bottom of the test file.

// fakeChannelForHook implements the narrow interface needed by hooksrv.
type fakeChannelForHook struct{ messages []string }

func (f *fakeChannelForHook) SendMessage(_ context.Context, _ int64, text string) error {
	f.messages = append(f.messages, text)
	return nil
}

// fakeApproverForHook queues a decision after a short delay to simulate async HITL.
type fakeApproverForHook struct {
	decision  hitl.HITLDecision
	delay     time.Duration
}

func (a *fakeApproverForHook) RequestPermission(_ context.Context, req hitl.PermissionRequest) error {
	go func() {
		time.Sleep(a.delay)
		req.DecisionCh <- a.decision
	}()
	return nil
}

func (a *fakeApproverForHook) RequestQuestion(_ context.Context, _ hitl.QuestionRequest) error {
	return nil
}

// pickFreePort returns an available TCP port on localhost.
func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pickFreePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestHookSrv_PreTool_Allow(t *testing.T) {
	port := pickFreePort(t)
	listenAddr := "127.0.0.1:" + itoa(port)

	approver := &fakeApproverForHook{
		decision: hitl.HITLDecision{Allow: true},
		delay:    50 * time.Millisecond,
	}
	ch := &fakeChannelForHook{}
	chatID := int64(123)
	srv := claudecode.NewHookServer(listenAddr, chatID, approver, ch)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(ctx, listenAddr)
	}()
	// Give server time to start.
	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(map[string]interface{}{
		"tool_name": "Edit",
		"tool_input": map[string]string{"file_path": "/tmp/foo.go"},
	})
	resp, err := http.Post("http://"+listenAddr+"/hook/pretool", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /hook/pretool: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	output, _ := result["hookSpecificOutput"].(map[string]interface{})
	if output == nil {
		t.Fatalf("hookSpecificOutput missing in response: %v", result)
	}
	if output["permissionDecision"] != "allow" {
		t.Errorf("permissionDecision: got %v, want allow", output["permissionDecision"])
	}

	cancel()
}

func TestHookSrv_PreTool_Deny(t *testing.T) {
	port := pickFreePort(t)
	listenAddr := "127.0.0.1:" + itoa(port)

	approver := &fakeApproverForHook{
		decision: hitl.HITLDecision{Allow: false},
		delay:    50 * time.Millisecond,
	}
	ch := &fakeChannelForHook{}
	srv := claudecode.NewHookServer(listenAddr, 123, approver, ch)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go srv.ListenAndServe(ctx, listenAddr)
	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(map[string]string{"tool_name": "Bash"})
	resp, err := http.Post("http://"+listenAddr+"/hook/pretool", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /hook/pretool: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	output, _ := result["hookSpecificOutput"].(map[string]interface{})
	if output == nil {
		t.Fatalf("hookSpecificOutput missing: %v", result)
	}
	if output["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision: got %v, want deny", output["permissionDecision"])
	}
	if _, ok := result["systemMessage"]; !ok {
		t.Error("systemMessage must be present on deny")
	}

	cancel()
}

func TestHookSrv_PreTool_Timeout_AutoDeny(t *testing.T) {
	port := pickFreePort(t)
	listenAddr := "127.0.0.1:" + itoa(port)

	// Approver that never resolves — simulates HITL timeout.
	approver := &neverApprover{}
	ch := &fakeChannelForHook{}
	srv := claudecode.NewHookServerWithTimeout(listenAddr, 123, approver, ch, 200*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go srv.ListenAndServe(ctx, listenAddr)
	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(map[string]string{"tool_name": "Bash"})
	start := time.Now()
	resp, err := http.Post("http://"+listenAddr+"/hook/pretool", "application/json", bytes.NewReader(body))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("POST /hook/pretool: %v", err)
	}
	defer resp.Body.Close()

	if elapsed < 150*time.Millisecond {
		t.Errorf("expected to wait for timeout (~200ms), returned in %v", elapsed)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	output, _ := result["hookSpecificOutput"].(map[string]interface{})
	if output["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision on timeout: got %v, want deny", output["permissionDecision"])
	}

	cancel()
}

func TestHookSrv_Notification_ForwardsToChannel(t *testing.T) {
	port := pickFreePort(t)
	listenAddr := "127.0.0.1:" + itoa(port)

	ch := &fakeChannelForHook{}
	srv := claudecode.NewHookServer(listenAddr, 123, &fakeApproverForHook{}, ch)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go srv.ListenAndServe(ctx, listenAddr)
	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(map[string]string{"message": "tool finished"})
	resp, err := http.Post("http://"+listenAddr+"/hook/notification", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /hook/notification: %v", err)
	}
	resp.Body.Close()

	time.Sleep(20 * time.Millisecond)
	if len(ch.messages) == 0 {
		t.Error("expected channel message, got none")
	}

	cancel()
}

// neverApprover never sends on DecisionCh (simulates timeout).
type neverApprover struct{}

func (a *neverApprover) RequestPermission(_ context.Context, _ hitl.PermissionRequest) error {
	return nil // never signals decisionCh
}
func (a *neverApprover) RequestQuestion(_ context.Context, _ hitl.QuestionRequest) error {
	return nil
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/claudecode/...
```

Expected: `FAIL` — `claudecode.NewHookServer` does not exist yet.

**Step 3: Write implementation**

```go
// internal/claudecode/hooksrv.go
package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/canhta/gistclaw/internal/hitl"
)

const defaultHITLTimeout = 5 * time.Minute

// hookSender is the narrow channel interface needed by the hook server.
type hookSender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// hookApprover is the narrow HITL interface needed by the hook server.
type hookApprover interface {
	RequestPermission(ctx context.Context, req hitl.PermissionRequest) error
	RequestQuestion(ctx context.Context, req hitl.QuestionRequest) error
}

// HookServer is the HTTP server at 127.0.0.1:8765 that gistclaw-hook calls back into.
// It is long-lived: started once in claudecode.Service.Run() and shared across tasks.
// Call SetChatID before each task to route responses to the correct Telegram chat.
type HookServer struct {
	addr        string
	mu          sync.RWMutex
	chatID      int64
	approver    hookApprover
	channel     hookSender
	hitlTimeout time.Duration
}

// SetChatID updates the Telegram chat ID used for HITL routing.
// Call this before starting each new task.
func (s *HookServer) SetChatID(chatID int64) {
	s.mu.Lock()
	s.chatID = chatID
	s.mu.Unlock()
}

func (s *HookServer) getChatID() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chatID
}

// NewHookServer constructs a HookServer with the default HITL timeout (5 minutes).
// addr is used only as a metadata field for construction; the actual listen address
// is passed to ListenAndServe.
func NewHookServer(addr string, chatID int64, approver hookApprover, ch hookSender) *HookServer {
	return NewHookServerWithTimeout(addr, chatID, approver, ch, defaultHITLTimeout)
}

// NewHookServerWithTimeout constructs a HookServer with a custom HITL timeout.
// Use this in tests to avoid waiting 5 minutes.
func NewHookServerWithTimeout(addr string, chatID int64, approver hookApprover, ch hookSender, timeout time.Duration) *HookServer {
	return &HookServer{
		addr:        addr,
		chatID:      chatID,
		approver:    approver,
		channel:     ch,
		hitlTimeout: timeout,
	}
}

// ListenAndServe starts the HTTP server and blocks until ctx is cancelled.
func (s *HookServer) ListenAndServe(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/hook/pretool", s.handlePreTool)
	mux.HandleFunc("/hook/notification", s.handleNotification)
	mux.HandleFunc("/hook/stop", s.handleStop)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("hooksrv: listen %s: %w", addr, err)
	}

	srv := &http.Server{Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return nil
	case err := <-errCh:
		return err
	}
}

// handlePreTool handles POST /hook/pretool.
// It blocks until the HITL decision is resolved or the timeout fires.
func (s *HookServer) handlePreTool(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Warn().Err(err).Msg("hooksrv: read pretool body")
		s.writeDeny(w, "failed to read request body")
		return
	}

	// Extract tool name and input for display to operator.
	var hookEvent struct {
		ToolName  string          `json:"tool_name"`
		ToolInput json.RawMessage `json:"tool_input"`
	}
	_ = json.Unmarshal(body, &hookEvent)

	decisionCh := make(chan hitl.HITLDecision, 1)
	permID := fmt.Sprintf("permission_%d", time.Now().UnixNano())
	req := hitl.PermissionRequest{
		ChatID:     s.getChatID(),
		ID:         permID,
		Permission: hookEvent.ToolName,
		Patterns:   []string{string(hookEvent.ToolInput)},
		DecisionCh: decisionCh,
	}

	if err := s.approver.RequestPermission(r.Context(), req); err != nil {
		log.Warn().Err(err).Msg("hooksrv: RequestPermission")
		s.writeDeny(w, "HITL request failed")
		return
	}

	select {
	case d := <-decisionCh:
		if d.Allow {
			s.writeAllow(w)
		} else {
			s.writeDeny(w, "Rejected by operator")
		}
	case <-time.After(s.hitlTimeout):
		log.Warn().Msg("hooksrv: HITL timeout — auto-deny")
		s.writeDeny(w, "HITL timeout — auto-denied")
	case <-r.Context().Done():
		s.writeDeny(w, "request cancelled")
	}
}

// handleNotification handles POST /hook/notification.
// Forwards the notification message to the Telegram channel.
func (s *HookServer) handleNotification(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var notif struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &notif)
	if notif.Message != "" {
		_ = s.channel.SendMessage(r.Context(), s.getChatID(), notif.Message)
	}
	w.WriteHeader(http.StatusOK)
}

// handleStop handles POST /hook/stop.
// Notifies the operator and is used by the service FSM to detect subprocess exit.
func (s *HookServer) handleStop(w http.ResponseWriter, r *http.Request) {
	_ = s.channel.SendMessage(r.Context(), s.getChatID(), "Claude Code session stopped.")
	w.WriteHeader(http.StatusOK)
}

// writeAllow writes a 200 response with permissionDecision=allow.
func (s *HookServer) writeAllow(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"hookSpecificOutput": map[string]string{
			"permissionDecision": "allow",
		},
	})
}

// writeDeny writes a 200 response with permissionDecision=deny.
// gistclaw-hook will interpret the deny and exit with code 2.
func (s *HookServer) writeDeny(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"hookSpecificOutput": map[string]string{
			"permissionDecision": "deny",
		},
		"systemMessage": reason,
	})
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/claudecode/... -run TestHookSrv -v -timeout 30s
```

Expected: `PASS` for all `TestHookSrv_*` tests.

**Step 5: Commit**

```bash
git add internal/claudecode/hooksrv.go internal/claudecode/hooksrv_test.go
git commit -m "feat(claudecode): implement hook HTTP server with HITL blocking"
```

---

### Task 5: `internal/claudecode/service.go` — Claude Code subprocess + FSM

**Files:**
- Create: `internal/claudecode/service.go`
- Create: `internal/claudecode/service_test.go`

**FSM states:**
```
Idle → Running (SubmitTask called)
Running → WaitingInput (permission.asked hook received)
WaitingInput → Running (permission resolved)
Running → Idle (subprocess exits cleanly)
```

**Step 1: Write the failing test**

```go
// internal/claudecode/service_test.go
package claudecode_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/claudecode"
	"github.com/canhta/gistclaw/internal/hitl"
)

// --- fakes shared within this test file ---
// (fakeChannelForHook, fakeApproverForHook, neverApprover, pickFreePort, itoa are defined in hooksrv_test.go)

type claudecodeFakeCostGuard struct{ tracked float64 }

func (g *claudecodeFakeCostGuard) Track(usd float64) { g.tracked += usd }

type claudecodeFakeSOULLoader struct{}

func (s *claudecodeFakeSOULLoader) Load() (string, error) { return "you are a helpful agent", nil }

// ---

// newEchoScript writes a shell script to a temp dir that mimics claude -p
// by printing stream-json lines to stdout and exiting cleanly.
func newEchoScript(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-claude")
	content := "#!/bin/sh\n"
	for _, l := range lines {
		// Use printf to avoid shell interpretation issues with JSON.
		content += "printf '%s\\n' " + "'" + strings.ReplaceAll(l, "'", "'\\''") + "'\n"
	}
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("write fake-claude: %v", err)
	}
	return script
}

// checkFakeClaudeAvailable skips the test if the system cannot run shell scripts.
func checkFakeClaudeAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
}

func TestClaudeCodeService_SubmitTask_StreamsText(t *testing.T) {
	checkFakeClaudeAvailable(t)

	lines := []string{
		`{"type":"text","text":"Hello "}`,
		`{"type":"text","text":"world!"}`,
		`{"type":"result","total_cost_usd":0.005,"is_error":false,"result":"done"}`,
	}
	fakeClaude := newEchoScript(t, lines)

	ch := &fakeChannelForHook{}
	guard := &claudecodeFakeCostGuard{}
	soul := &claudecodeFakeSOULLoader{}
	approver := &fakeApproverForHook{decision: hitl.HITLDecision{Allow: true}, delay: 0}

	cfg := claudecode.Config{
		Dir:            t.TempDir(),
		HookServerAddr: "127.0.0.1:" + itoa(pickFreePort(t)),
	}

	svc := claudecode.New(cfg, fakeClaude, ch, approver, guard, soul)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := svc.SubmitTask(ctx, 123, "build the auth module"); err != nil {
		t.Fatalf("SubmitTask: unexpected error: %v", err)
	}

	full := strings.Join(ch.messages, "")
	if !strings.Contains(full, "Hello world!") {
		t.Errorf("expected streamed text, got: %v", ch.messages)
	}
	if guard.tracked != 0.005 {
		t.Errorf("CostGuard.Track: got %v, want 0.005", guard.tracked)
	}
	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "✅ Done") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ✅ Done message, got: %v", ch.messages)
	}
}

func TestClaudeCodeService_SubmitTask_ZeroOutput(t *testing.T) {
	checkFakeClaudeAvailable(t)

	lines := []string{
		`{"type":"result","total_cost_usd":0,"is_error":false,"result":""}`,
	}
	fakeClaude := newEchoScript(t, lines)

	ch := &fakeChannelForHook{}
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:" + itoa(pickFreePort(t))},
		fakeClaude, ch, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = svc.SubmitTask(ctx, 123, "test")

	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "no output") || strings.Contains(m, "⚠️") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected zero-output warning, got: %v", ch.messages)
	}
}

func TestClaudeCodeService_SubmitTask_ErrorResult(t *testing.T) {
	checkFakeClaudeAvailable(t)

	lines := []string{
		`{"type":"result","total_cost_usd":0,"is_error":true,"result":"claude exploded"}`,
	}
	fakeClaude := newEchoScript(t, lines)

	ch := &fakeChannelForHook{}
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:" + itoa(pickFreePort(t))},
		fakeClaude, ch, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = svc.SubmitTask(ctx, 123, "test")

	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "claude exploded") || strings.Contains(m, "⚠️") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error message, got: %v", ch.messages)
	}
}

func TestClaudeCodeService_Busy_RejectsSecondTask(t *testing.T) {
	// Uses a script that blocks for 500ms.
	checkFakeClaudeAvailable(t)

	dir := t.TempDir()
	script := filepath.Join(dir, "slow-claude")
	slowScript := "#!/bin/sh\nsleep 0.5\nprintf '{\"type\":\"result\",\"total_cost_usd\":0,\"is_error\":false,\"result\":\"ok\"}\\n'\n"
	if err := os.WriteFile(script, []byte(slowScript), 0755); err != nil {
		t.Fatalf("write slow-claude: %v", err)
	}

	ch := &fakeChannelForHook{}
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:" + itoa(pickFreePort(t))},
		script, ch, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start first task in background.
	go svc.SubmitTask(ctx, 123, "first")
	time.Sleep(100 * time.Millisecond) // give first task time to start

	// Second task while first is running.
	_ = svc.SubmitTask(ctx, 123, "second")

	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "busy") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected busy message for second task, got: %v", ch.messages)
	}
}

func TestClaudeCodeService_Name(t *testing.T) {
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:8765"},
		"claude", &fakeChannelForHook{}, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)
	if svc.Name() != "claudecode" {
		t.Errorf("Name: got %q, want claudecode", svc.Name())
	}
}

func TestClaudeCodeService_IsAlive_IdleIsAlive(t *testing.T) {
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:8765"},
		"claude", &fakeChannelForHook{}, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)
	// Idle state with no subprocess = alive (service is ready to accept work).
	ctx := context.Background()
	if !svc.IsAlive(ctx) {
		t.Error("IsAlive: expected true in Idle state")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/claudecode/... -run TestClaudeCodeService -v
```

Expected: `FAIL` — `claudecode.New` does not exist yet.

**Step 3: Write implementation**

```go
// internal/claudecode/service.go
package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/hitl"
)

// Narrow dependency interfaces — keeps claudecodeServiceImpl testable without
// importing infra directly. The concrete infra types satisfy these interfaces;
// tests provide lightweight fakes.

type claudecodeApprover interface {
	RequestPermission(ctx context.Context, req hitl.PermissionRequest) error
	RequestQuestion(ctx context.Context, req hitl.QuestionRequest) error
}

type claudecodeCostTracker interface {
	Track(usd float64)
}

type claudecodeSoulLoader interface {
	Load() (string, error)
}

// fsmState is the FSM state for claudecode.Service.
type fsmState int32

const (
	fsmIdle         fsmState = iota // No subprocess running; ready for new task.
	fsmRunning                      // Subprocess running; processing output.
	fsmWaitingInput                 // Subprocess blocked on hook/pretool approval.
)

// Service is the interface satisfied by *serviceImpl.
// It satisfies infra.AgentHealthChecker via Name() + IsAlive().
type Service interface {
	Run(ctx context.Context) error
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	Stop(ctx context.Context) error
	IsAlive(ctx context.Context) bool
	Name() string // returns "claudecode"
}

// claudecodeServiceImpl implements Service.
type claudecodeServiceImpl struct {
	cfg         Config
	claudeBin   string // path/name of the claude binary (injected for testing)
	ch          channel.Channel
	approver    claudecodeApprover
	guard       claudecodeCostTracker
	soul        claudecodeSoulLoader

	state   fsmState   // accessed via atomic
	mu      sync.Mutex
	cmd     *exec.Cmd  // current subprocess; nil when Idle
	hookSrv *HookServer // long-lived hook server; started in Run()
}

// New constructs a claudecode.Service. All dependencies are injected as interfaces.
// In production, pass *infra.CostGuard and *infra.SOULLoader (both satisfy the narrow
// interfaces above). In tests, pass lightweight fakes.
// claudeBin is the path or name of the claude binary. In tests, pass the path to a
// fake script. In production, pass "claude" (resolved via PATH).
func New(cfg Config, claudeBin string, ch channel.Channel, approver claudecodeApprover, guard claudecodeCostTracker, soul claudecodeSoulLoader) Service {
	if cfg.HookServerAddr == "" {
		cfg.HookServerAddr = "127.0.0.1:8765"
	}
	return &claudecodeServiceImpl{
		cfg:       cfg,
		claudeBin: claudeBin,
		ch:        ch,
		approver:  approver,
		guard:     guard,
		soul:      soul,
	}
}

func (s *claudecodeServiceImpl) Name() string { return "claudecode" }

// Run starts the long-lived hook HTTP server and blocks until ctx is cancelled.
// The hook server is started once here (not per-task) so Claude Code can call
// gistclaw-hook at any point after Run() begins without a race condition.
func (s *claudecodeServiceImpl) Run(ctx context.Context) error {
	s.hookSrv = NewHookServer(s.cfg.HookServerAddr, 0, s.approver, s.ch)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.hookSrv.ListenAndServe(ctx, s.cfg.HookServerAddr)
	}()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// IsAlive returns true if the FSM is in any active state (Idle, Running, or WaitingInput).
// The service is considered alive as long as it has not crashed. A nil subprocess in
// Idle state means the service is alive and ready to accept work.
func (s *claudecodeServiceImpl) IsAlive(_ context.Context) bool {
	return true // FSM is always "alive" — not crashed; subprocess may or may not be running
}

// SubmitTask starts a `claude -p` subprocess and streams output to Telegram.
func (s *claudecodeServiceImpl) SubmitTask(ctx context.Context, chatID int64, prompt string) error {
	// Check FSM: reject if already running.
	if !atomic.CompareAndSwapInt32((*int32)(&s.state), int32(fsmIdle), int32(fsmRunning)) {
		_ = s.ch.SendMessage(ctx, chatID, "⚠️ Claude Code is busy. Wait for the current task to finish.")
		return nil
	}
	defer atomic.StoreInt32((*int32)(&s.state), int32(fsmIdle))

	// Merge .claude/settings.local.json with hook configuration.
	if err := s.patchSettings(); err != nil {
		log.Warn().Err(err).Msg("claudecode: could not patch settings.local.json; hooks may not work")
	}

	// Load SOUL.md.
	soul, err := s.soul.Load()
	if err != nil {
		log.Warn().Err(err).Msg("claudecode: could not load SOUL.md; proceeding without system prompt")
		soul = ""
	}

	// Build args.
	args := []string{"-p", prompt, "--output-format", "stream-json"}
	if soul != "" {
		args = append(args, "--system-prompt", soul)
	}

	cmd := exec.CommandContext(ctx, s.claudeBin, args...)
	cmd.Dir = s.cfg.Dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("claudecode: stdout pipe: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		s.mu.Lock()
		s.cmd = nil
		s.mu.Unlock()
		return fmt.Errorf("claudecode: start subprocess: %w", err)
	}

	// Update the long-lived hook server's chatID for this task.
	// The hook server was started in Run() and is shared across tasks.
	if s.hookSrv != nil {
		s.hookSrv.SetChatID(chatID)
	}

	// Parse stream-json output.	var buf strings.Builder
	hadOutput := false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		ev, err := ParseStreamLine(line)
		if err != nil {
			log.Warn().Err(err).Msg("claudecode: skip malformed stream-json line")
			continue
		}
		if ev == nil {
			continue
		}

		switch ev.Type {
		case "text":
			hadOutput = true
			buf.WriteString(ev.Text)
			// Flush at 4096-char boundary.
			for buf.Len() >= 4096 {
				chunk := buf.String()[:4096]
				_ = s.ch.SendMessage(ctx, chatID, chunk)
				rest := buf.String()[4096:]
				buf.Reset()
				buf.WriteString(rest)
			}

		case "result":
			s.guard.Track(ev.TotalCostUSD)
			// Flush remaining buffer.
			if buf.Len() > 0 {
				_ = s.ch.SendMessage(ctx, chatID, buf.String())
				buf.Reset()
				hadOutput = true
			}
			if ev.IsError {
				msg := ev.Result
				if msg == "" {
					msg = "Claude Code encountered an error."
				}
				_ = s.ch.SendMessage(ctx, chatID, "⚠️ "+msg)
			} else if !hadOutput {
				_ = s.ch.SendMessage(ctx, chatID, "⚠️ Agent finished but produced no output.")
			} else {
				_ = s.ch.SendMessage(ctx, chatID, "✅ Done")
			}
		}
	}

	// Subprocess finished.
	_ = cmd.Wait()

	s.mu.Lock()
	s.cmd = nil
	s.mu.Unlock()

	return scanner.Err()
}

// Stop sends SIGTERM to the subprocess, waits 2s, then sends SIGKILL.
func (s *claudecodeServiceImpl) Stop(_ context.Context) error {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		log.Warn().Err(err).Msg("claudecode: SIGTERM")
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Exited cleanly after SIGTERM.
	case <-time.After(2 * time.Second):
		log.Warn().Msg("claudecode: subprocess did not exit after SIGTERM; sending SIGKILL")
		_ = cmd.Process.Kill()
	}
	return nil
}

// patchSettings merges hook configuration into .claude/settings.local.json.
func (s *claudecodeServiceImpl) patchSettings() error {
	settingsPath := s.cfg.SettingsPath
	if settingsPath == "" {
		settingsPath = filepath.Join(s.cfg.Dir, ".claude", "settings.local.json")
	}

	// Backup.
	existing, err := os.ReadFile(settingsPath)
	if err == nil {
		_ = os.WriteFile("/tmp/gistclaw-claude-settings.bak", existing, 0600)
	}

	// Read existing (or start with {}).
	var settings map[string]interface{}
	if err == nil {
		if jsonErr := json.Unmarshal(existing, &settings); jsonErr != nil {
			settings = map[string]interface{}{}
		}
	} else {
		settings = map[string]interface{}{}
	}

	// Patch only the hooks key.
	settings["hooks"] = map[string]interface{}{
		"PreToolUse": []map[string]interface{}{
			{
				"matcher": ".*",
				"hooks": []map[string]string{
					{"type": "command", "command": "gistclaw-hook"},
				},
			},
		},
		"PostToolUse": []map[string]interface{}{
			{
				"matcher": ".*",
				"hooks": []map[string]string{
					{"type": "command", "command": "gistclaw-hook --type notification"},
				},
			},
		},
		"Stop": []map[string]interface{}{
			{
				"hooks": []map[string]string{
					{"type": "command", "command": "gistclaw-hook --type stop"},
				},
			},
		},
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(settingsPath), err)
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return os.WriteFile(settingsPath, out, 0644)
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/claudecode/... -v -timeout 30s
```

Expected: `PASS` for all tests. Tests that need the `sh` binary will auto-skip on platforms without it.

**Step 5: Commit**

```bash
git add internal/claudecode/service.go internal/claudecode/service_test.go
git commit -m "feat(claudecode): implement Claude Code subprocess service with FSM"
```

---

### Task 6: `cmd/gistclaw-hook/main.go` — hook helper binary

**Files:**
- Create: `cmd/gistclaw-hook/main.go`
- Create: `cmd/gistclaw-hook/main_test.go`

**Behavior:**
1. Parse flags: `--type` (default `"pretool"`), `--addr` (default `"127.0.0.1:8765"`)
2. Read JSON from stdin
3. `POST http://<addr>/hook/<type>` with the stdin JSON as body
4. Block waiting for HTTP response (up to 6 minutes)
5. On response:
   - If `hookSpecificOutput.permissionDecision == "allow"`: write JSON body to stdout, exit 0
   - Else: write JSON body to stderr, exit 2
6. On timeout or network error: write deny JSON to stderr, exit 2

**Step 1: Write the failing test**

```go
// cmd/gistclaw-hook/main_test.go
package main_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildHook compiles the gistclaw-hook binary into a temp directory.
func buildHook(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell integration test: skip on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "gistclaw-hook")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/canhta/gistclaw/cmd/gistclaw-hook")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build gistclaw-hook: %v", err)
	}
	return bin
}

func TestHookBinary_Allow_ExitsZero(t *testing.T) {
	bin := buildHook(t)

	// Mock server that returns allow.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"hookSpecificOutput": map[string]string{
				"permissionDecision": "allow",
			},
		})
	}))
	defer srv.Close()

	// Extract addr from srv.URL (strip "http://").
	addr := strings.TrimPrefix(srv.URL, "http://")

	input := `{"tool_name":"Edit","tool_input":{"file_path":"/tmp/foo.go"}}`
	cmd := exec.Command(bin, "--addr", addr)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("gistclaw-hook exited with error (expected 0): %v\nstderr: %s", err, stderr.String())
	}
	if stdout.String() == "" {
		t.Error("expected allow JSON on stdout, got nothing")
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &out); err != nil {
		t.Errorf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
}

func TestHookBinary_Deny_ExitsTwo(t *testing.T) {
	bin := buildHook(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"hookSpecificOutput": map[string]string{
				"permissionDecision": "deny",
			},
			"systemMessage": "Rejected by operator",
		})
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	cmd := exec.Command(bin, "--addr", addr)
	cmd.Stdin = strings.NewReader(`{"tool_name":"Bash"}`)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit code for deny, got 0")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %T", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("exit code: got %d, want 2", exitErr.ExitCode())
	}
	if stderr.String() == "" {
		t.Error("expected deny JSON on stderr, got nothing")
	}
}

func TestHookBinary_NetworkError_ExitsTwo(t *testing.T) {
	bin := buildHook(t)

	// Use an address where nothing is listening.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--addr", "127.0.0.1:19998")
	cmd.Stdin = strings.NewReader(`{"tool_name":"Bash"}`)

	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit for network error, got 0")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %T — %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("exit code: got %d, want 2", exitErr.ExitCode())
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./cmd/gistclaw-hook/...
```

Expected: `FAIL` — the package does not exist yet; `go build` inside the test will fail.

**Step 3: Write implementation**

```go
// cmd/gistclaw-hook/main.go
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// hookTimeout is the maximum time gistclaw-hook will wait for a response from
// the gistclaw hook server. Claude Code's default hook timeout is 10 minutes;
// we use 6 minutes to allow the operator time to respond with a small margin.
const hookTimeout = 6 * time.Minute

func main() {
	hookType := flag.String("type", "pretool", "Hook type: pretool | notification | stop")
	addr := flag.String("addr", "127.0.0.1:8765", "Address of the gistclaw hook server")
	flag.Parse()

	// Read JSON from stdin.
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		writeStderr(denyResponse("failed to read stdin: " + err.Error()))
		os.Exit(2)
	}

	// POST to the hook server.
	url := "http://" + *addr + "/hook/" + *hookType
	client := &http.Client{Timeout: hookTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(input))
	if err != nil {
		writeStderr(denyResponse("hook server unreachable: " + err.Error()))
		os.Exit(2)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeStderr(denyResponse("failed to read response: " + err.Error()))
		os.Exit(2)
	}

	// For non-pretool types, just exit 0 — no decision needed.
	if *hookType != "pretool" {
		os.Exit(0)
	}

	// Parse decision from response.
	var result struct {
		HookSpecificOutput struct {
			PermissionDecision string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
		SystemMessage string `json:"systemMessage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		writeStderr(denyResponse("malformed response from hook server: " + err.Error()))
		os.Exit(2)
	}

	if result.HookSpecificOutput.PermissionDecision == "allow" {
		fmt.Fprint(os.Stdout, string(body))
		os.Exit(0)
	}

	// Deny.
	writeStderr(body)
	os.Exit(2)
}

// denyResponse returns a JSON deny payload as a byte slice.
func denyResponse(reason string) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"hookSpecificOutput": map[string]string{
			"permissionDecision": "deny",
		},
		"systemMessage": reason,
	})
	return b
}

// writeStderr writes data to stderr, ignoring write errors.
func writeStderr(data []byte) {
	_, _ = os.Stderr.Write(data)
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./cmd/gistclaw-hook/... -v -timeout 60s
```

Expected: `PASS` for all three tests. The `TestHookBinary_NetworkError_ExitsTwo` test should complete quickly since `net.Dial` to a closed port fails immediately with a connection refused error.

**Step 5: Run full test suite to confirm no regressions**

```bash
go test ./... -timeout 60s
```

Expected: `PASS` for all packages.

**Step 6: Commit**

```bash
git add cmd/gistclaw-hook/main.go cmd/gistclaw-hook/main_test.go
git commit -m "feat(gistclaw-hook): implement hook helper binary (stdin→POST→stdout/stderr)"
```

---

## Final verification

After all six tasks are committed, run the full suite one more time:

```bash
go build ./...
go test ./... -timeout 120s
```

Expected: all packages compile, all tests pass.

---

Plan 6 complete. Next: Plan 7 (Scheduler & Gateway).
