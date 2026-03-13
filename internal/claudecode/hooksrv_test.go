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

// fakeChannelForHook implements the narrow interface needed by hooksrv.
type fakeChannelForHook struct{ messages []string }

func (f *fakeChannelForHook) SendMessage(_ context.Context, _ int64, text string) error {
	f.messages = append(f.messages, text)
	return nil
}

// fakeApproverForHook queues a decision after a short delay to simulate async HITL.
type fakeApproverForHook struct {
	decision hitl.HITLDecision
	delay    time.Duration
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
	_ = l.Close()
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
		"tool_name":  "Edit",
		"tool_input": map[string]string{"file_path": "/tmp/foo.go"},
	})
	resp, err := http.Post("http://"+listenAddr+"/hook/pretool", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /hook/pretool: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

	go func() { _ = srv.ListenAndServe(ctx, listenAddr) }()
	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(map[string]string{"tool_name": "Bash"})
	resp, err := http.Post("http://"+listenAddr+"/hook/pretool", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /hook/pretool: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

	go func() { _ = srv.ListenAndServe(ctx, listenAddr) }()
	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(map[string]string{"tool_name": "Bash"})
	start := time.Now()
	resp, err := http.Post("http://"+listenAddr+"/hook/pretool", "application/json", bytes.NewReader(body))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("POST /hook/pretool: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

	go func() { _ = srv.ListenAndServe(ctx, listenAddr) }()
	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(map[string]string{"message": "tool finished"})
	resp, err := http.Post("http://"+listenAddr+"/hook/notification", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /hook/notification: %v", err)
	}
	_ = resp.Body.Close()

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
