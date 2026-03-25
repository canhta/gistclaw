package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
	"github.com/canhta/gistclaw/internal/store"
)

type ContextAssembler interface {
	Assemble(ctx context.Context, input ContextAssemblyInput) (GenerateRequest, error)
}

type ContextAssemblyInput struct {
	SessionID     string
	AgentID       string
	Agent         model.AgentProfile
	Objective     string
	WorkspaceRoot string
	MemoryView    memory.ContextView
}

type defaultContextAssembler struct {
	store         *store.DB
	convStore     *conversations.ConversationStore
	workspaceLoad WorkspaceContextLoader
}

func newDefaultContextAssembler(
	db *store.DB,
	cs *conversations.ConversationStore,
	workspaceLoad WorkspaceContextLoader,
) *defaultContextAssembler {
	if workspaceLoad == nil {
		workspaceLoad = newWorkspaceContextLoader()
	}
	return &defaultContextAssembler{
		store:         db,
		convStore:     cs,
		workspaceLoad: workspaceLoad,
	}
}

func (a *defaultContextAssembler) Assemble(ctx context.Context, input ContextAssemblyInput) (GenerateRequest, error) {
	workspace, err := a.workspaceLoad.Load(ctx, input.WorkspaceRoot)
	if err != nil {
		return GenerateRequest{}, fmt.Errorf("assemble provider request: workspace context: %w", err)
	}

	req := GenerateRequest{
		Instructions: composeInstructions(input.Objective, input.Agent, input.MemoryView, workspace),
	}
	if input.SessionID == "" {
		return req, nil
	}

	_, mailbox, err := sessions.NewService(a.store, a.convStore).LoadSessionMailbox(ctx, input.SessionID, 100)
	if err == sessions.ErrSessionNotFound {
		return req, nil
	}
	if err != nil {
		return GenerateRequest{}, fmt.Errorf("assemble provider request: load session mailbox: %w", err)
	}
	req.ConversationCtx = mailboxToEvents(mailbox)
	return req, nil
}

func composeInstructions(objective string, agent model.AgentProfile, contextView memory.ContextView, workspace WorkspaceContext) string {
	parts := []string{"Objective:\n" + objective}

	agentParts := make([]string, 0, 6)
	if agent.AgentID != "" {
		agentParts = append(agentParts, "Agent ID: "+agent.AgentID)
	}
	if agent.Role != "" {
		agentParts = append(agentParts, "Role: "+agent.Role)
	}
	if agent.ToolProfile != "" {
		agentParts = append(agentParts, "Tool posture: "+agent.ToolProfile)
	}
	if len(agent.CanSpawn) > 0 {
		agentParts = append(agentParts, "Can spawn: "+strings.Join(agent.CanSpawn, ", "))
	}
	if agent.Instructions != "" {
		agentParts = append(agentParts, "Rules:\n"+agent.Instructions)
	}
	if len(agentParts) > 0 {
		parts = append(parts, "Agent contract:\n"+strings.Join(agentParts, "\n"))
	}

	if workspace.Root != "" {
		workspaceParts := []string{"Workspace root:\n" + workspace.Root}
		if len(workspace.Tree) > 0 {
			workspaceParts = append(workspaceParts, "Workspace tree:\n"+strings.Join(workspace.Tree, "\n"))
		}
		if len(workspace.Files) > 0 {
			fileBlocks := make([]string, 0, len(workspace.Files))
			for _, file := range workspace.Files {
				fileBlocks = append(fileBlocks, renderWorkspaceFileBlock(file.Path, file.Content))
			}
			workspaceParts = append(workspaceParts, "Workspace context:\n"+strings.Join(fileBlocks, "\n\n"))
		}
		parts = append(parts, strings.Join(workspaceParts, "\n\n"))
	}

	if contextView.Summary.Content != "" {
		parts = append(parts, "Working summary:\n"+contextView.Summary.Content)
	}
	if len(contextView.Items) > 0 {
		facts := make([]string, 0, len(contextView.Items))
		for _, item := range contextView.Items {
			if item.Content == "" {
				continue
			}
			facts = append(facts, "- "+item.Content)
		}
		if len(facts) > 0 {
			parts = append(parts, "Memory facts:\n"+strings.Join(facts, "\n"))
		}
	}

	return strings.Join(parts, "\n\n")
}
