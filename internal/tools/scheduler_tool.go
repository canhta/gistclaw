package tools

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/scheduler"
)

// schedulerTool wraps a single scheduler operation as a Tool.
type schedulerTool struct {
	def    providers.Tool
	execFn func(ctx context.Context, input map[string]any) (string, error)
}

func (s *schedulerTool) Definition() providers.Tool { return s.def }

func (s *schedulerTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	result, err := s.execFn(ctx, input)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("%s error: %v", s.def.Name, err)}
	}
	return ToolResult{ForLLM: result}
}

// NewSchedulerTools returns Tool instances for all four scheduler operations.
func NewSchedulerTools(sched *scheduler.Service) []Tool {
	schTools := sched.Tools()
	result := make([]Tool, 0, len(schTools))
	for _, def := range schTools {
		d := def // capture
		var execFn func(ctx context.Context, input map[string]any) (string, error)
		switch d.Name {
		case "schedule_job":
			execFn = func(_ context.Context, input map[string]any) (string, error) {
				kind, _ := input["kind"].(string)
				target, _ := input["target"].(string)
				prompt, _ := input["prompt"].(string)
				schedule, _ := input["schedule"].(string)
				agentKind, err := agent.KindFromString(target)
				if err != nil {
					return "", fmt.Errorf("schedule_job: invalid target %q: %w", target, err)
				}
				j := scheduler.Job{
					Kind:     kind,
					Target:   agentKind,
					Prompt:   prompt,
					Schedule: schedule,
				}
				if err := sched.CreateJob(j); err != nil {
					return "", err
				}
				return `{"status":"created"}`, nil
			}
		case "list_jobs":
			execFn = func(_ context.Context, _ map[string]any) (string, error) {
				jobs, err := sched.ListJobs()
				if err != nil {
					return "", err
				}
				return scheduler.JobsToJSON(jobs), nil
			}
		case "update_job":
			execFn = func(_ context.Context, input map[string]any) (string, error) {
				id, _ := input["id"].(string)
				if id == "" {
					return "", fmt.Errorf("update_job: id is required")
				}
				fields := make(map[string]any)
				for k, v := range input {
					if k != "id" {
						fields[k] = v
					}
				}
				if err := sched.UpdateJob(id, fields); err != nil {
					return "", err
				}
				return `{"status":"updated"}`, nil
			}
		case "delete_job":
			execFn = func(_ context.Context, input map[string]any) (string, error) {
				id, _ := input["id"].(string)
				if id == "" {
					return "", fmt.Errorf("delete_job: id is required")
				}
				if err := sched.DeleteJob(id); err != nil {
					return "", err
				}
				return `{"status":"deleted"}`, nil
			}
		default:
			execFn = func(_ context.Context, _ map[string]any) (string, error) {
				return "", fmt.Errorf("unknown scheduler tool: %q", d.Name)
			}
		}
		result = append(result, &schedulerTool{def: d, execFn: execFn})
	}
	return result
}
