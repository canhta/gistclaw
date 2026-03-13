// internal/opencode/service_test.go
package opencode_test

import (
	"context"
	"encoding/json"
	"errors"
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
func (f *fakeChannel) SendTyping(_ context.Context, _ int64) error { return nil }
func (f *fakeChannel) Name() string                                { return "fake" }
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
			_, _ = fmt.Fprintln(w, line)
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
	_, _ = fmt.Sscanf(portStr, "%d", &port)

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
	_, _ = fmt.Sscanf(portStr, "%d", &port)

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
	_, _ = fmt.Sscanf(portStr, "%d", &port)

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
	_, _ = fmt.Sscanf(portStr, "%d", &port)

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
	_, _ = fmt.Sscanf(portStr, "%d", &port)

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

func TestSubmitTaskWithResult_ReturnsAccumulatedText(t *testing.T) {
	var promptCalled atomic.Bool
	sseLines := []string{
		`data: {"type":"message.part.updated","part":{"type":"text","text":"Result "}}`,
		`data: {"type":"message.part.updated","part":{"type":"text","text":"chunk two"}}`,
		`data: {"type":"session.status","status":{"type":"idle"}}`,
	}
	srv := newMockOpenCodeServer(t, sseLines, &promptCalled)
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	portStr := parts[len(parts)-1]
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	ch := &fakeChannel{}
	svc := opencode.New(opencode.Config{Port: port, Dir: t.TempDir()},
		ch, &fakeApprover{}, &fakeCostGuard{}, &fakeSOULLoader{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := svc.SubmitTaskWithResult(ctx, 123, "summarise the project")
	if err != nil {
		t.Fatalf("SubmitTaskWithResult: unexpected error: %v", err)
	}

	if !promptCalled.Load() {
		t.Error("expected POST /session/:id/prompt_async to be called")
	}

	// Returned string must contain the full concatenated text.
	want := "Result chunk two"
	if result != want {
		t.Errorf("SubmitTaskWithResult: got %q, want %q", result, want)
	}

	// Should also have streamed to Telegram normally (text + ✅ Done).
	full := strings.Join(ch.messages, "")
	if !strings.Contains(full, "Result chunk two") {
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

func TestSubmitTaskWithResult_BusyReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/global/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "sess_busy_02"})
	})
	mux.HandleFunc("/session/sess_busy_02/prompt_async", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"session is busy"}`, http.StatusConflict)
	})
	mux.HandleFunc("/event", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// emit nothing — should not be reached
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	portStr := parts[len(parts)-1]
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	ch := &fakeChannel{}
	svc := opencode.New(opencode.Config{Port: port, Dir: t.TempDir()},
		ch, &fakeApprover{}, &fakeCostGuard{}, &fakeSOULLoader{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := svc.SubmitTaskWithResult(ctx, 123, "another task while busy")
	if !errors.Is(err, opencode.ErrSessionBusy) {
		t.Errorf("SubmitTaskWithResult busy: got %v, want opencode.ErrSessionBusy", err)
	}
}
