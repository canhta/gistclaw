package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/providers"
)

// rememberTool appends a fact to MEMORY.md.
type rememberTool struct{ eng memory.Engine }

func NewRememberTool(eng memory.Engine) Tool { return &rememberTool{eng: eng} }
func (r *rememberTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "remember",
		Description: "Save a fact to long-term memory (MEMORY.md).",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"content": map[string]any{"type": "string"}},
			"required":   []string{"content"},
		},
	}
}
func (r *rememberTool) Execute(_ context.Context, input map[string]any) ToolResult {
	content, _ := input["content"].(string)
	if content == "" {
		return ToolResult{ForLLM: "remember: content is required"}
	}
	if err := r.eng.AppendFact(content); err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("remember error: %v", err)}
	}
	return ToolResult{ForLLM: `{"status":"ok"}`, ForUser: "Remembered."}
}

// noteTool appends an entry to today's notes file.
type noteTool struct{ eng memory.Engine }

func NewNoteTool(eng memory.Engine) Tool { return &noteTool{eng: eng} }
func (n *noteTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "note",
		Description: "Add an entry to today's daily notes.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"content": map[string]any{"type": "string"}},
			"required":   []string{"content"},
		},
	}
}
func (n *noteTool) Execute(_ context.Context, input map[string]any) ToolResult {
	content, _ := input["content"].(string)
	if content == "" {
		return ToolResult{ForLLM: "note: content is required"}
	}
	if err := n.eng.AppendNote(content); err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("note error: %v", err)}
	}
	return ToolResult{ForLLM: `{"status":"ok"}`, ForUser: "Noted."}
}

// curateMemoryTool reviews and rewrites MEMORY.md via an LLM call.
type curateMemoryTool struct {
	eng memory.Engine
	llm providers.LLMProvider
}

func NewCurateMemoryTool(eng memory.Engine, llm providers.LLMProvider) Tool {
	return &curateMemoryTool{eng: eng, llm: llm}
}
func (c *curateMemoryTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "curate_memory",
		Description: "Review and rewrite MEMORY.md to remove stale or redundant entries.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}
func (c *curateMemoryTool) Execute(ctx context.Context, _ map[string]any) ToolResult {
	current, err := os.ReadFile(c.eng.MemoryPath())
	if err != nil || len(current) == 0 {
		return ToolResult{ForLLM: "memory is empty; nothing to curate"}
	}
	prompt := fmt.Sprintf(
		"You are a memory curator. Review the following memory entries and rewrite them "+
			"as a concise, deduplicated list of facts. Remove stale or redundant entries. "+
			"Return only the rewritten memory content, no commentary.\n\n%s", current)
	resp, err := c.llm.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("curate_memory failed: %v", err)}
	}
	if resp == nil || resp.Content == "" {
		return ToolResult{ForLLM: "curate_memory: LLM returned no content"}
	}
	if err := c.eng.Rewrite(resp.Content); err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("curate_memory write error: %v", err)}
	}
	return ToolResult{ForLLM: `{"status":"ok","message":"Memory curated."}`, ForUser: "Memory curated."}
}
