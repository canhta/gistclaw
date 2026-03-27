package control

import (
	"context"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

type stubInspector struct {
	status runtime.ConversationStatus
	err    error
	key    conversations.ConversationKey
	reset  runtime.ConversationResetOutcome
}

func (s *stubInspector) InspectConversation(_ context.Context, key conversations.ConversationKey) (runtime.ConversationStatus, error) {
	s.key = key
	return s.status, s.err
}

func (s *stubInspector) ResetConversation(_ context.Context, key conversations.ConversationKey) (runtime.ConversationResetOutcome, error) {
	s.key = key
	return s.reset, s.err
}

func TestDispatcherDispatch(t *testing.T) {
	tests := []struct {
		name         string
		env          model.Envelope
		status       runtime.ConversationStatus
		wantHandled  bool
		wantContains []string
		wantKey      conversations.ConversationKey
	}{
		{
			name: "help command",
			env: model.Envelope{
				ConnectorID:    "telegram",
				AccountID:      "acct-1",
				ConversationID: "chat-1",
				ThreadID:       "main",
				Text:           "/help",
			},
			wantHandled:  true,
			wantContains: []string{"Message me naturally", "/status", "/reset"},
		},
		{
			name: "help command localized",
			env: model.Envelope{
				ConnectorID:    "telegram",
				AccountID:      "acct-1",
				ConversationID: "chat-1",
				ThreadID:       "main",
				Text:           "/help",
				Metadata: map[string]string{
					"language_hint": "vi",
				},
			},
			wantHandled:  true,
			wantContains: []string{"Nhắn cho mình tự nhiên", "/status", "/reset"},
		},
		{
			name: "status command",
			env: model.Envelope{
				ConnectorID:    "whatsapp",
				AccountID:      "acct-2",
				ConversationID: "chat-2",
				ThreadID:       "main",
				Text:           "/status",
			},
			status: runtime.ConversationStatus{
				Exists: true,
				ActiveRun: model.Run{
					ID:        "run-active-1234",
					Objective: "review the repo",
					Status:    model.RunStatusActive,
				},
				ActiveGate: model.ConversationGate{
					ID:    "gate-1",
					Title: "Approval required for shell_exec",
				},
				PendingGateCount: 2,
			},
			wantHandled:  true,
			wantContains: []string{"Active run", "run-acti", "Waiting for your reply", "Approval required for shell_exec", "2 pending decisions"},
			wantKey: conversations.ConversationKey{
				ConnectorID: "whatsapp",
				AccountID:   "acct-2",
				ExternalID:  "chat-2",
				ThreadID:    "main",
			},
		},
		{
			name: "status command localized",
			env: model.Envelope{
				ConnectorID:    "telegram",
				AccountID:      "acct-2",
				ConversationID: "chat-2",
				ThreadID:       "main",
				Text:           "/status",
				Metadata: map[string]string{
					"language_hint": "vi",
				},
			},
			status: runtime.ConversationStatus{
				Exists: true,
				ActiveRun: model.Run{
					ID:        "run-active-1234",
					Objective: "review the repo",
					Status:    model.RunStatusActive,
				},
				ActiveGate: model.ConversationGate{
					ID:    "gate-1",
					Title: "Approval required for shell_exec",
				},
				PendingGateCount: 2,
			},
			wantHandled:  true,
			wantContains: []string{"Tiến trình đang chạy", "run-acti", "Đang chờ bạn trả lời", "Approval required for shell_exec", "2 quyết định đang chờ"},
			wantKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-2",
				ExternalID:  "chat-2",
				ThreadID:    "main",
			},
		},
		{
			name: "plain chat not handled",
			env: model.Envelope{
				ConnectorID:    "telegram",
				AccountID:      "acct-3",
				ConversationID: "chat-3",
				ThreadID:       "main",
				Text:           "review the repo",
			},
			wantHandled: false,
		},
		{
			name: "reset command success",
			env: model.Envelope{
				ConnectorID:    "telegram",
				AccountID:      "acct-4",
				ConversationID: "chat-4",
				ThreadID:       "main",
				Text:           "/reset",
			},
			wantHandled:  true,
			wantContains: []string{"Chat reset", "History cleared"},
			wantKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-4",
				ExternalID:  "chat-4",
				ThreadID:    "main",
			},
		},
		{
			name: "reset command localized",
			env: model.Envelope{
				ConnectorID:    "telegram",
				AccountID:      "acct-4",
				ConversationID: "chat-4",
				ThreadID:       "main",
				Text:           "/reset",
				Metadata: map[string]string{
					"language_hint": "vi",
				},
			},
			wantHandled:  true,
			wantContains: []string{"Đã đặt lại chat", "Lịch sử"},
			wantKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-4",
				ExternalID:  "chat-4",
				ThreadID:    "main",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inspector := &stubInspector{status: tt.status, reset: runtime.ConversationResetCleared}
			dispatcher := NewDispatcher(inspector)

			reply, handled, err := dispatcher.Dispatch(context.Background(), tt.env)
			if err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			if handled != tt.wantHandled {
				t.Fatalf("handled = %v, want %v", handled, tt.wantHandled)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(reply, want) {
					t.Fatalf("expected reply to include %q, got:\n%s", want, reply)
				}
			}
			if tt.wantKey != (conversations.ConversationKey{}) && inspector.key != tt.wantKey {
				t.Fatalf("expected inspect key %+v, got %+v", tt.wantKey, inspector.key)
			}
		})
	}
}

func TestDispatcherDispatchResetOutcomeMessages(t *testing.T) {
	tests := []struct {
		name         string
		outcome      runtime.ConversationResetOutcome
		err          error
		wantContains []string
	}{
		{
			name:         "missing conversation",
			outcome:      runtime.ConversationResetMissing,
			wantContains: []string{"Nothing to reset"},
		},
		{
			name:         "busy conversation",
			outcome:      runtime.ConversationResetBusy,
			wantContains: []string{"active run", "Retry /reset"},
		},
		{
			name:    "busy conversation localized",
			outcome: runtime.ConversationResetBusy,
			wantContains: []string{
				"tiến trình hoạt động",
				"Hãy thử /reset",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher := NewDispatcher(&stubInspector{reset: tt.outcome, err: tt.err})
			env := model.Envelope{
				ConnectorID:    "telegram",
				AccountID:      "acct-1",
				ConversationID: "chat-1",
				ThreadID:       "main",
				Text:           "/reset",
			}
			if strings.Contains(tt.name, "localized") {
				env.Metadata = map[string]string{"language_hint": "vi"}
			}
			reply, handled, err := dispatcher.Dispatch(context.Background(), env)
			if err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			if !handled {
				t.Fatal("expected reset to be handled")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(reply, want) {
					t.Fatalf("expected reply to include %q, got:\n%s", want, reply)
				}
			}
		})
	}
}
