package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/debugrpc"
	"github.com/canhta/gistclaw/internal/runtime/capabilities"
)

func (a *App) CapabilityAppAction(ctx context.Context, req capabilities.AppActionRequest) (capabilities.AppActionResult, error) {
	if a == nil {
		return capabilities.AppActionResult{}, fmt.Errorf("app capabilities: app is required")
	}

	switch strings.TrimSpace(req.Name) {
	case debugrpc.ProbeStatus:
		status, err := a.InspectStatus(ctx)
		if err != nil {
			return capabilities.AppActionResult{}, fmt.Errorf("app capabilities: status: %w", err)
		}
		return capabilities.AppActionResult{
			Name:    "status",
			Summary: "system status loaded",
			Data: map[string]any{
				"active_runs":       status.ActiveRuns,
				"interrupted_runs":  status.InterruptedRuns,
				"pending_approvals": status.PendingApprovals,
				"storage_backup":    status.Storage.BackupStatus,
				"storage_warnings":  status.Storage.Warnings,
			},
		}, nil
	case debugrpc.ProbeConnectorHealth:
		result, err := a.connectorHealthActionResult(ctx)
		if err != nil {
			return capabilities.AppActionResult{}, fmt.Errorf("app capabilities: connector health: %w", err)
		}
		return result, nil
	case debugrpc.ProbeActiveProject:
		result, err := a.activeProjectActionResult(ctx)
		if err != nil {
			return capabilities.AppActionResult{}, fmt.Errorf("app capabilities: active project: %w", err)
		}
		return result, nil
	case debugrpc.ProbeScheduleStatus:
		result, err := a.scheduleStatusActionResult(ctx)
		if err != nil {
			return capabilities.AppActionResult{}, fmt.Errorf("app capabilities: schedule status: %w", err)
		}
		return result, nil
	default:
		return capabilities.AppActionResult{}, fmt.Errorf("app capabilities: unsupported action %q", strings.TrimSpace(req.Name))
	}
}
