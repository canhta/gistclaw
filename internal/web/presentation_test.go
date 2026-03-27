package web

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestSessionPresentationUsesAgentLanguage(t *testing.T) {
	t.Parallel()

	if got := sessionRoleLabel(model.SessionRoleFront); got != "Lead agent" {
		t.Fatalf("expected lead role label, got %q", got)
	}
	if got := sessionRoleLabel(model.SessionRoleWorker); got != "Specialist agent" {
		t.Fatalf("expected specialist role label, got %q", got)
	}
	if got := sessionRoleSummaryLabel(model.SessionRoleFront); got != "Lead agent" {
		t.Fatalf("expected lead role summary, got %q", got)
	}
	if got := sessionRoleSummaryLabel(model.SessionRoleWorker); got != "Specialist agent" {
		t.Fatalf("expected specialist role summary, got %q", got)
	}
	if got := sessionMessageKindLabel(model.MessageAnnounce); got != "Note" {
		t.Fatalf("expected announcement label, got %q", got)
	}
	if got := sessionMessageKindLabel(model.MessageAgentSend); got != "Agent reply" {
		t.Fatalf("expected agent-send label, got %q", got)
	}
	if got := sessionSenderLabel(""); got != "You / GistClaw" {
		t.Fatalf("expected local sender label, got %q", got)
	}
}
