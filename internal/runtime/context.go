package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	recommendationpkg "github.com/canhta/gistclaw/internal/runtime/recommendation"
	"github.com/canhta/gistclaw/internal/sessions"
	"github.com/canhta/gistclaw/internal/store"
)

type ContextAssembler interface {
	Assemble(ctx context.Context, input ContextAssemblyInput) (GenerateRequest, error)
}

type ContextAssemblyInput struct {
	SessionID               string
	AgentID                 string
	Agent                   model.AgentProfile
	Specialists             map[string]model.AgentProfile
	ToolSpecs               []model.ToolSpec
	Objective               string
	CWD                     string
	MemoryView              memory.ContextView
	ExecutionRecommendation recommendationpkg.Decision
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
		Instructions: composeInstructions(
			input.Objective,
			input.Agent,
			input.Specialists,
			input.ToolSpecs,
			input.ExecutionRecommendation,
			input.MemoryView,
			directory,
		),
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

func composeInstructions(
	objective string,
	agent model.AgentProfile,
	specialists map[string]model.AgentProfile,
	toolSpecs []model.ToolSpec,
	executionRecommendation recommendationpkg.Decision,
	contextView memory.ContextView,
	directory DirectoryContext,
) string {
	parts := []string{"Objective:\n" + objective}

	agentParts := make([]string, 0, 6)
	if agent.AgentID != "" {
		agentParts = append(agentParts, "Agent ID: "+agent.AgentID)
	}
	if agent.Role != "" {
		agentParts = append(agentParts, "Role: "+agent.Role)
	}
	if agent.BaseProfile != "" {
		agentParts = append(agentParts, "Base profile: "+string(agent.BaseProfile))
	}
	if len(agent.ToolFamilies) > 0 {
		agentParts = append(agentParts, "Tool families: "+joinToolFamilies(agent.ToolFamilies))
	}
	if len(agent.DelegationKinds) > 0 {
		agentParts = append(agentParts, "Delegation kinds: "+joinDelegationKinds(agent.DelegationKinds))
	}
	if agent.SpecialistSummaryVisibility != "" {
		agentParts = append(agentParts, "Specialist visibility: "+string(agent.SpecialistSummaryVisibility))
	}
	if len(agent.CanMessage) > 0 {
		agentParts = append(agentParts, "Can message: "+strings.Join(agent.CanMessage, ", "))
	}
	if agent.Instructions != "" {
		agentParts = append(agentParts, "Rules:\n"+agent.Instructions)
	}
	if len(agentParts) > 0 {
		parts = append(parts, "Agent contract:\n"+strings.Join(agentParts, "\n"))
	}
	if recommendationBlock := renderExecutionRecommendation(executionRecommendation); recommendationBlock != "" {
		parts = append(parts, recommendationBlock)
	}
	if capabilityBlock := renderDirectCapabilityGuidance(executionRecommendation, toolSpecs); capabilityBlock != "" {
		parts = append(parts, capabilityBlock)
	}
	if specialistsBlock := renderSpecialistRoster(agent.SpecialistSummaryVisibility, specialists); specialistsBlock != "" {
		parts = append(parts, specialistsBlock)
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

func renderDirectCapabilityGuidance(decision recommendationpkg.Decision, toolSpecs []model.ToolSpec) string {
	if decision.Mode != recommendationpkg.ModeDirect || len(toolSpecs) == 0 {
		return ""
	}

	names := make([]string, 0, len(toolSpecs))
	seen := make(map[string]bool, len(toolSpecs))
	for _, spec := range toolSpecs {
		if spec.Family != model.ToolFamilyConnectorCapability && spec.Family != model.ToolFamilyRuntimeCapability {
			continue
		}
		if seen[spec.Name] {
			continue
		}
		seen[spec.Name] = true
		names = append(names, spec.Name)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)

	lines := make([]string, 0, len(names)+2)
	lines = append(lines, "Direct capability tools available:")
	for _, name := range names {
		lines = append(lines, "- "+name)
	}
	lines = append(lines, "Use these before delegation for bounded local or connector tasks.")
	return strings.Join(lines, "\n")
}

func renderExecutionRecommendation(decision recommendationpkg.Decision) string {
	if decision.Mode == "" {
		return ""
	}
	lines := []string{
		"Execution recommendation:",
		"Mode: " + string(decision.Mode),
	}
	if decision.Rationale != "" {
		lines = append(lines, "Rationale: "+decision.Rationale)
	}
	if decision.Confidence > 0 {
		lines = append(lines, fmt.Sprintf("Confidence: %.2f", decision.Confidence))
	}
	if len(decision.SuggestedKinds) > 0 {
		lines = append(lines, "Suggested delegation kinds: "+joinDelegationKinds(decision.SuggestedKinds))
	}
	return strings.Join(lines, "\n")
}

func renderSpecialistRoster(
	visibility model.SpecialistSummaryVisibility,
	specialists map[string]model.AgentProfile,
) string {
	if visibility == model.SpecialistSummaryNone || len(specialists) == 0 {
		return ""
	}
	ids := make([]string, 0, len(specialists))
	for id := range specialists {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	lines := make([]string, 0, len(ids)+1)
	lines = append(lines, "Specialists available:")
	for _, id := range ids {
		specialist := specialists[id]
		switch visibility {
		case model.SpecialistSummaryFull:
			line := fmt.Sprintf("- %s (%s)", id, specialist.BaseProfile)
			if specialist.Role != "" {
				line += ": " + specialist.Role
			}
			if len(specialist.ToolFamilies) > 0 {
				line += " [tools: " + joinToolFamilies(specialist.ToolFamilies) + "]"
			}
			if len(specialist.DelegationKinds) > 0 {
				line += " [delegation: " + joinDelegationKinds(specialist.DelegationKinds) + "]"
			}
			lines = append(lines, line)
		default:
			lines = append(lines, fmt.Sprintf("- %s (%s)", id, specialist.BaseProfile))
		}
	}
	return strings.Join(lines, "\n")
}

func joinToolFamilies(values []model.ToolFamily) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, string(value))
	}
	return strings.Join(items, ", ")
}

func joinDelegationKinds(values []model.DelegationKind) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, string(value))
	}
	return strings.Join(items, ", ")
}
