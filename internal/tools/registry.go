package tools

import (
	"context"

	"github.com/canhta/gistclaw/internal/model"
)

type Tool interface {
	Name() string
	Spec() model.ToolSpec
	Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) List() []model.ToolSpec {
	specs := make([]model.ToolSpec, 0, len(r.tools))
	for _, tool := range r.tools {
		specs = append(specs, tool.Spec())
	}
	return specs
}
