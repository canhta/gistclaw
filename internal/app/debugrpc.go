package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/debugrpc"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/runtime/capabilities"
)

func (a *App) DebugRPCStatus(ctx context.Context, selected string) (debugrpc.Status, error) {
	probes := debugrpc.Catalog()
	result, err := a.DebugRPCProbe(ctx, selected)
	if err != nil {
		return debugrpc.Status{}, err
	}

	return debugrpc.Status{
		Summary: debugrpc.Summary{
			ProbeCount:    len(probes),
			ReadOnly:      true,
			DefaultProbe:  debugrpc.ProbeStatus,
			SelectedProbe: result.Probe,
		},
		Probes: probes,
		Result: result,
	}, nil
}

func (a *App) DebugRPCProbe(ctx context.Context, selected string) (debugrpc.Result, error) {
	if a == nil {
		return debugrpc.Result{}, fmt.Errorf("debug rpc: app is required")
	}

	probe, ok := debugrpc.ResolveProbe(strings.TrimSpace(selected))
	if !ok {
		return debugrpc.Result{}, fmt.Errorf("%w: %s", debugrpc.ErrUnknownProbe, strings.TrimSpace(selected))
	}

	action, err := a.CapabilityAppAction(ctx, capabilities.AppActionRequest{Name: probe.Name})
	if err != nil {
		return debugrpc.Result{}, fmt.Errorf("debug rpc: run probe %q: %w", probe.Name, err)
	}

	executedAt := time.Now().UTC()
	return debugrpc.Result{
		Probe:           probe.Name,
		Label:           probe.Label,
		Summary:         action.Summary,
		ExecutedAt:      executedAt.Format(time.RFC3339),
		ExecutedAtLabel: executedAt.Format("2006-01-02 15:04:05 MST"),
		Data:            action.Data,
	}, nil
}

func (a *App) connectorHealthActionResult(ctx context.Context) (capabilities.AppActionResult, error) {
	health, err := a.ConnectorHealth(ctx)
	if err != nil {
		return capabilities.AppActionResult{}, fmt.Errorf("load connector health: %w", err)
	}

	connectors := make([]map[string]any, 0, len(health))
	healthyCount := 0
	degradedCount := 0
	unknownCount := 0
	for _, snapshot := range health {
		state := string(snapshot.State)
		switch snapshot.State {
		case model.ConnectorHealthHealthy:
			healthyCount++
		case model.ConnectorHealthDegraded:
			degradedCount++
		default:
			unknownCount++
			if strings.TrimSpace(state) == "" {
				state = string(model.ConnectorHealthUnknown)
			}
		}

		item := map[string]any{
			"connector_id":      snapshot.ConnectorID,
			"state":             state,
			"summary":           snapshot.Summary,
			"restart_suggested": snapshot.RestartSuggested,
		}
		if !snapshot.CheckedAt.IsZero() {
			item["checked_at"] = snapshot.CheckedAt.UTC().Format(time.RFC3339)
		}
		connectors = append(connectors, item)
	}

	return capabilities.AppActionResult{
		Name:    debugrpc.ProbeConnectorHealth,
		Summary: fmt.Sprintf("%d connector snapshots loaded", len(connectors)),
		Data: map[string]any{
			"summary": map[string]any{
				"total":    len(connectors),
				"healthy":  healthyCount,
				"degraded": degradedCount,
				"unknown":  unknownCount,
			},
			"connectors": connectors,
		},
	}, nil
}

func (a *App) activeProjectActionResult(ctx context.Context) (capabilities.AppActionResult, error) {
	project, err := runtime.ActiveProject(ctx, a.db)
	if err != nil {
		return capabilities.AppActionResult{}, fmt.Errorf("load active project: %w", err)
	}

	return capabilities.AppActionResult{
		Name:    debugrpc.ProbeActiveProject,
		Summary: "active project loaded",
		Data: map[string]any{
			"id":     project.ID,
			"name":   project.Name,
			"path":   project.PrimaryPath,
			"source": project.Source,
		},
	}, nil
}

func (a *App) scheduleStatusActionResult(ctx context.Context) (capabilities.AppActionResult, error) {
	status, err := a.ScheduleStatus(ctx)
	if err != nil {
		return capabilities.AppActionResult{}, fmt.Errorf("load schedule status: %w", err)
	}

	data := map[string]any{
		"enabled":            status.Enabled,
		"total_schedules":    status.TotalSchedules,
		"enabled_schedules":  status.EnabledSchedules,
		"due_schedules":      status.DueSchedules,
		"active_occurrences": status.ActiveOccurrences,
	}
	if !status.NextWakeAt.IsZero() {
		data["next_wake_at"] = status.NextWakeAt.UTC().Format(time.RFC3339)
	}
	if status.LastFailure != nil {
		data["last_failure"] = map[string]any{
			"schedule_id": status.LastFailure.ScheduleID,
			"name":        status.LastFailure.Name,
			"error":       status.LastFailure.Error,
			"failed_at":   status.LastFailure.FailedAt.UTC().Format(time.RFC3339),
		}
	}

	return capabilities.AppActionResult{
		Name:    debugrpc.ProbeScheduleStatus,
		Summary: fmt.Sprintf("%d schedules configured", status.TotalSchedules),
		Data:    data,
	}, nil
}
