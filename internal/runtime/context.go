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
	SessionID  string
	AgentID    string
	Agent      model.AgentProfile
	Objective  string
	CWD        string
	MemoryView memory.ContextView
}

type defaultContextAssembler struct {
	store         *store.DB
	convStore     *conversations.ConversationStore
	directoryLoad DirectoryContextLoader
}

func newDefaultContextAssembler(
	db *store.DB,
	cs *conversations.ConversationStore,
	directoryLoad DirectoryContextLoader,
) *defaultContextAssembler {
	if directoryLoad == nil {
		directoryLoad = newDirectoryContextLoader()
	}
	return &defaultContextAssembler{
		store:         db,
		convStore:     cs,
		directoryLoad: directoryLoad,
	}
}

func (a *defaultContextAssembler) Assemble(ctx context.Context, input ContextAssemblyInput) (GenerateRequest, error) {
	directory, err := a.directoryLoad.Load(ctx, input.CWD)
	if err != nil {
		return GenerateRequest{}, fmt.Errorf("assemble provider request: directory context: %w", err)
	}

	req := GenerateRequest{
		Instructions: composeInstructions(input.Objective, input.Agent, input.MemoryView, directory),
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

func composeInstructions(objective string, agent model.AgentProfile, contextView memory.ContextView, directory DirectoryContext) string {
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

	if directory.Root != "" {
		directoryParts := []string{"Working directory:\n" + directory.Root}
		if len(directory.Tree) > 0 {
			directoryParts = append(directoryParts, "Directory tree:\n"+strings.Join(directory.Tree, "\n"))
		}
		if len(directory.Files) > 0 {
			fileBlocks := make([]string, 0, len(directory.Files))
			for _, file := range directory.Files {
				fileBlocks = append(fileBlocks, renderDirectoryFileBlock(file.Path, file.Content))
			}
			directoryParts = append(directoryParts, "Directory context:\n"+strings.Join(fileBlocks, "\n\n"))
		}
		parts = append(parts, strings.Join(directoryParts, "\n\n"))
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
