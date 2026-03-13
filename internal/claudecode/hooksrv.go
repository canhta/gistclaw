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
