package runtime

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
)

func TestScopeConversationKey_UsesExplicitCWDRegistration(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})

	key, project, err := rt.scopeConversationKey(context.Background(), conversations.ConversationKey{
		ConnectorID: "telegram",
		ThreadID:    "thread-1",
	}, "", "/tmp/project-alpha")
	if err != nil {
		t.Fatalf("scopeConversationKey: %v", err)
	}
	if key.ProjectID == "" {
		t.Fatal("expected scoped key to bind a project id")
	}
	if project.ID != key.ProjectID {
		t.Fatalf("project id = %q, want %q", project.ID, key.ProjectID)
	}
	if project.PrimaryPath != "/tmp/project-alpha" {
		t.Fatalf("primary_path = %q, want %q", project.PrimaryPath, "/tmp/project-alpha")
	}
}

func TestScopeConversationKey_UsesActiveProjectWhenExplicitScopeMissing(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	project, err := ActivateProjectPath(context.Background(), db, "/tmp/project-alpha", "alpha", "operator")
	if err != nil {
		t.Fatalf("ActivateProjectPath: %v", err)
	}
	if err := SetActiveProject(context.Background(), db, project.ID); err != nil {
		t.Fatalf("SetActiveProject: %v", err)
	}

	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	key, scopedProject, err := rt.scopeConversationKey(context.Background(), conversations.ConversationKey{
		ConnectorID: "telegram",
		ThreadID:    "thread-1",
	}, "", "")
	if err != nil {
		t.Fatalf("scopeConversationKey: %v", err)
	}
	if key.ProjectID != project.ID {
		t.Fatalf("project id = %q, want %q", key.ProjectID, project.ID)
	}
	if scopedProject.ID != project.ID {
		t.Fatalf("scoped project id = %q, want %q", scopedProject.ID, project.ID)
	}
}
